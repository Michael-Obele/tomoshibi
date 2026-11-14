package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	// bogdanfinn/websocket (already a dependency) speaks bogdanfinn/fhttp, not
	// net/http, so it cannot upgrade a net/http route. coder/websocket is
	// net/http-native and has zero transitive dependencies.
	"github.com/coder/websocket"
)

// Server is the bridge listener. It hosts at most one connected extension
// (the newest connection wins; a browser restart must not leave a zombie in
// charge), routes fetch requests to it, and audits every access.
type Server struct {
	state *State
	log   *slog.Logger

	mu      sync.Mutex
	conn    *client
	pending map[string]chan Message
}

// AllowsOrigin reports whether the bridge may drive an origin without a recipe.
// The transport asks this before it touches the browser: the allowlist is what
// replaced the recipe as the borrow lane's gate.
func (s *Server) AllowsOrigin(origin string) bool { return s.state.Allows(origin) }

// AllowedOrigins returns the recipe-free allowlist (for the CLI and status).
func (s *Server) AllowedOrigins() []string { return s.state.AllowedOrigins() }

// State exposes the pairing state, so the transport can audit with the same
// writer and a CLI can mutate the allowlist under the same lock.
func (s *Server) State() *State { return s.state }

type client struct {
	conn    *websocket.Conn
	remote  string
	writeMu sync.Mutex
	// paired and nonce are touched from the connection's read goroutine and
	// from Fetch, so both are guarded by Server.mu.
	paired   bool
	nonce    string
	tokenSHA string // which device this connection authenticated as
	browser  string
	version  string
	pairTry  int
	// closed is closed when the connection goes away, so a fetch waiting on a
	// dead browser fails immediately instead of burning its whole timeout.
	closed chan struct{}
}

// NewServer builds the bridge listener over a state dir.
func NewServer(state *State, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		state:   state,
		log:     logger,
		pending: map[string]chan Message{},
	}
}

// Start rotates the pairing code on a ticker until ctx is done, so a stale
// code cannot sit in a screenshot of the terminal forever.
func (s *Server) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(CodeTTL / 2)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.state.Code() // rotates when stale
			}
		}
	}()
}

// Code exposes the current pairing code for the API's startup log.
func (s *Server) Code() string { return s.state.Code() }

// ServeWS upgrades one loopback connection and serves it until it closes.
// Mounted outside the /v1 auth group: the extension is not an API-key client.
func (s *Server) ServeWS(w http.ResponseWriter, r *http.Request) {
	if !remoteIsLoopback(r) {
		s.state.Audit(AuditEvent{Event: "refuse", Note: "non-loopback connection from " + r.RemoteAddr})
		http.Error(w, "bridge: loopback only", http.StatusForbidden)
		return
	}
	if !hostIsLoopback(r.Host) {
		s.state.Audit(AuditEvent{Event: "refuse", Note: "non-loopback Host header " + r.Host})
		http.Error(w, "bridge: loopback only", http.StatusForbidden)
		return
	}
	// A page running in the user's own browser is also a loopback peer, so the
	// peer address alone is not enough: any http(s) Origin is refused, which
	// kills the "malicious page talks to the bridge" path (and with it DNS
	// rebinding, since the Origin is what the page has to declare). A browser
	// sends its Origin for cross-origin WebSockets; the extension's own origin
	// is chrome-extension://<id> or moz-extension://<uuid>, and non-browser
	// clients (curl, the CLI) send no Origin at all.
	if origin := r.Header.Get("Origin"); origin != "" {
		if !extensionOrigin(origin) {
			s.state.Audit(AuditEvent{Event: "refuse", Note: "origin refused: " + origin})
			http.Error(w, "bridge: extension origins only", http.StatusForbidden)
			return
		}
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"chrome-extension://*", "moz-extension://*"},
	})
	if err != nil {
		s.log.Warn("bridge: upgrade failed", "error", err)
		return
	}
	conn.SetReadLimit(MaxMessageBytes)

	c := &client{conn: conn, remote: r.RemoteAddr, closed: make(chan struct{})}
	s.adopt(c)
	defer func() {
		close(c.closed)
		s.drop(c)
	}()

	// Keepalive: an app-level ping every 20s also keeps an MV3 service worker
	// awake while the socket is open.
	pingCtx, stopPing := context.WithCancel(r.Context())
	defer stopPing()
	go s.keepalive(pingCtx, c)

	for {
		typ, raw, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			s.send(c, Message{Type: MsgError, Code: CodeRefused, Message: "malformed JSON frame"})
			continue
		}
		s.handle(c, msg)
	}
}

// Fetch implements the scraper's transport: it hands the request to the
// connected extension and waits for the browser's answer.
func (s *Server) Fetch(ctx context.Context, req FetchRequest) (FetchResult, error) {
	start := time.Now()

	s.mu.Lock()
	c := s.conn
	paired := c != nil && c.paired
	tokenSHA := ""
	if c != nil {
		tokenSHA = c.tokenSHA
	}
	s.mu.Unlock()
	if c == nil {
		return FetchResult{}, ErrOffline
	}
	if !paired {
		return FetchResult{}, ErrUnpaired
	}
	// Revocation has to reach a connection that is already up: `bridge unpair`
	// (or deleting the state file) clears the device list, and this is where
	// that lands. A browser with no device is no longer a browser we may drive.
	if !s.state.HasDevice(tokenSHA) {
		s.log.Info("bridge: dropping a connection whose device was revoked")
		c.conn.CloseNow()
		return FetchResult{}, ErrUnpaired
	}

	id := randomHex(8)
	ch := make(chan Message, 1)
	s.mu.Lock()
	s.pending[id] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}()

	timeout := 60 * time.Second
	if req.TimeoutMS > 0 {
		timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out := Message{
		Type: MsgFetch, ID: id, URL: req.URL, Actions: req.Actions,
		WaitSelector: req.WaitSelector, SettleMS: req.SettleMS,
		TimeoutMS: req.TimeoutMS, Screenshot: req.Screenshot, FullPage: req.FullPage,
		IO: req.IO, Recipe: req.Recipe, Lane: req.Lane,
	}
	if err := s.send(c, out); err != nil {
		return FetchResult{}, fmt.Errorf("bridge: send failed: %w", err)
	}

	select {
	case msg := <-ch:
		res := FetchResult{
			Status: msg.Status, FinalURL: msg.FinalURL, Title: msg.Title,
			Text: msg.Text, HTML: msg.HTML, HTMLTruncated: msg.HTMLTruncated,
			ScreenshotB64: msg.ScreenshotB64, Markers: msg.Markers,
			ActionsRun: msg.ActionsRun, IO: msg.IO, HTTPStatus: msg.HTTPStatus,
			Error: msg.Message, Ms: time.Since(start).Milliseconds(),
		}
		s.auditFetch(req, res, "")
		return res, nil

	case <-waitCtx.Done():
		// Tell the extension to abandon the tab work; it keeps the tab for the
		// human. The pending entry is removed by the deferred cleanup.
		s.send(c, Message{Type: MsgCancel, ID: id})
		s.auditFetch(req, FetchResult{IO: req.IO}, "timeout")
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			return FetchResult{}, ErrTimeout
		}
		return FetchResult{}, waitCtx.Err()

	case <-c.closed:
		// The browser went away mid-fetch (a crash, a reload, or a frame the
		// socket dropped): say so now instead of waiting out the timeout.
		s.auditFetch(req, FetchResult{IO: req.IO}, "disconnected")
		return FetchResult{}, ErrDisconnected
	}
}

// auditFetch writes one audit line. One helper so the timeout and answer paths
// cannot drift (the timeout line used to lose the io layer).
func (s *Server) auditFetch(req FetchRequest, res FetchResult, result string) {
	if result == "" {
		result = res.Status
	}
	ev := AuditEvent{
		Event: "fetch", Recipe: req.Recipe, Lane: req.Lane, IO: res.IO,
		Origin: originOf(req.URL), Actions: len(req.Actions),
		Pages: 1, Result: result, Ms: res.Ms,
	}
	if res.Error != "" {
		ev.Note = res.Error
	}
	s.state.Audit(ev)
}

func (s *Server) adopt(c *client) {
	s.mu.Lock()
	old := s.conn
	s.conn = c
	s.mu.Unlock()
	if old != nil {
		s.log.Info("bridge: a newer connection replaced the previous one")
		old.conn.CloseNow()
	}
	s.log.Info("bridge: extension connected", "remote", c.remote)
}

func (s *Server) drop(c *client) {
	s.mu.Lock()
	if s.conn == c {
		s.conn = nil
	}
	s.mu.Unlock()
	c.conn.CloseNow()
	s.log.Info("bridge: extension disconnected")
}

func (s *Server) keepalive(ctx context.Context, c *client) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.send(c, Message{Type: MsgPing}); err != nil {
				return
			}
		}
	}
}

func (s *Server) send(c *client, msg Message) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	// A fixed 10s write budget; a wedged socket is closed by its read loop
	// failing, not by elaborate write retry logic.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.conn.Write(ctx, websocket.MessageText, raw)
}

func (s *Server) handle(c *client, msg Message) {
	switch msg.Type {
	case MsgHello:
		if msg.Protocol != ProtocolVersion {
			s.send(c, Message{Type: MsgError, Code: CodeProtocolMismatch,
				Message: fmt.Sprintf("protocol %d expected, got %d", ProtocolVersion, msg.Protocol)})
			c.conn.CloseNow()
			return
		}
		s.mu.Lock()
		c.browser, c.version = msg.Browser, msg.Version
		c.nonce = randomHex(16)
		nonce := c.nonce
		s.mu.Unlock()
		// The challenge proves this is the Tomoshibi the extension paired
		// with; without it the extension must not release its token.
		s.send(c, Message{Type: MsgChallenge, Nonce: nonce, Proof: ServerProof(s.state.ServerKey(), nonce)})

	case MsgAuth:
		s.mu.Lock()
		want := c.nonce
		s.mu.Unlock()
		if want == "" || msg.Nonce != want {
			s.send(c, Message{Type: MsgError, Code: CodeRefused, Message: "stale handshake; reconnect"})
			c.conn.CloseNow()
			return
		}
		if s.state.ValidToken(msg.Token) {
			s.setPaired(c, true)
			s.mu.Lock()
			c.tokenSHA = DeviceTokenSHA(msg.Token)
			s.mu.Unlock()
			s.send(c, Message{Type: MsgStatus, Paired: true, Allowed: s.state.AllowedOrigins()})
			s.state.Audit(AuditEvent{Event: "hello", Note: "paired " + c.browser + " " + c.version})
			return
		}
		s.state.Audit(AuditEvent{Event: "refuse", Note: "stale token from " + c.browser})
		s.send(c, Message{Type: MsgPairRequired, Nonce: want})

	case MsgPair:
		s.mu.Lock()
		want := c.nonce
		c.pairTry++
		over := c.pairTry > 5
		s.mu.Unlock()
		if over {
			s.send(c, Message{Type: MsgError, Code: CodePairLocked, Message: "too many attempts on this connection"})
			c.conn.CloseNow()
			return
		}
		if want == "" || msg.Nonce != want {
			s.send(c, Message{Type: MsgError, Code: CodeRefused, Message: "stale handshake; reconnect"})
			c.conn.CloseNow()
			return
		}
		token, serverKey, err := s.state.Pair(msg.Code, c.browser)
		if err != nil {
			s.state.Audit(AuditEvent{Event: "refuse", Note: "pair failed"})
			s.send(c, Message{Type: MsgError, Code: CodePairFailed, Message: err.Error()})
			return
		}
		s.setPaired(c, true)
		s.mu.Lock()
		c.tokenSHA = DeviceTokenSHA(token)
		s.mu.Unlock()
		// The proof is keyed by the code the user just typed: a squatter on
		// the port cannot fake a successful pairing, and the extension refuses
		// to store anything it cannot verify.
		s.send(c, Message{
			Type: MsgPairOK, Token: token, ServerKey: serverKey, Nonce: want,
			Proof: PairProof(msg.Code, want, token, serverKey),
		})
		s.state.Audit(AuditEvent{Event: "pair", Note: c.browser + " " + c.version})
		s.log.Info("bridge: device paired", "browser", c.browser, "version", c.version)

	case MsgFetchResult:
		s.mu.Lock()
		paired := c.paired
		ch := s.pending[msg.ID]
		s.mu.Unlock()
		if !paired {
			return
		}
		if ch != nil {
			select {
			case ch <- msg:
			default: // late duplicate after timeout; drop
			}
		}

	case MsgPong:
		// liveness only

	case MsgStatus:
		// informational; nothing to route

	case MsgCancel:
		// server never receives cancel in v1 (it sends it); ignore politely

	default:
		s.send(c, Message{Type: MsgError, Code: CodeRefused, Message: "unknown message type: " + msg.Type})
	}
}

// setPaired records authentication state under the lock Fetch reads it with.
func (s *Server) setPaired(c *client, v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == c {
		c.paired = v
	}
}

// extensionOrigin reports whether an Origin header value can only come from a
// browser extension (chrome-extension://id, moz-extension://uuid).
func extensionOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "chrome-extension", "moz-extension", "safari-web-extension":
		return u.Host != ""
	default:
		return false
	}
}

// hostIsLoopback checks the Host header, so a proxied or rebinding request
// cannot claim to be local while looking like a public name.
func hostIsLoopback(host string) bool {
	if host == "" {
		return true // HTTP/1.0 style clients; the peer check already passed
	}
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// remoteIsLoopback refuses anything that is not a direct loopback peer. The
// proxy-header check matters because a tunnelled deployment would otherwise
// forward a remote client onto 127.0.0.1 and look local.
func remoteIsLoopback(r *http.Request) bool {
	if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Real-Ip") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + u.Host
}
