package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func newTestServer(t *testing.T) (*Server, *State, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := NewState(dir, nil)
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	srv := NewServer(st, nil)
	ts := httptest.NewServer(http.HandlerFunc(srv.ServeWS))
	t.Cleanup(ts.Close)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	return srv, st, wsURL
}

func dial(t *testing.T, wsURL string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return conn
}

func writeMsg(t *testing.T, conn *websocket.Conn, msg Message) {
	t.Helper()
	raw, _ := json.Marshal(msg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readMsg(t *testing.T, conn *websocket.Conn, timeout time.Duration) Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, raw, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return msg
}

// hello runs hello and returns the server's challenge.
func hello(t *testing.T, conn *websocket.Conn) Message {
	t.Helper()
	writeMsg(t, conn, Message{Type: MsgHello, Protocol: ProtocolVersion, Browser: "test", Version: "0.0.1"})
	got := readMsg(t, conn, 3*time.Second)
	if got.Type != MsgChallenge || got.Nonce == "" || got.Proof == "" {
		t.Fatalf("want challenge with nonce and proof, got %+v", got)
	}
	return got
}

// handshake runs hello -> pair and returns the device token plus the server key
// the extension would have stored. It also verifies the pairing proof the way
// the real extension does: with the code the user typed.
func handshake(t *testing.T, wsURL string, st *State) (*websocket.Conn, string) {
	t.Helper()
	conn := dial(t, wsURL)
	ch := hello(t, conn)
	// The code rotates on success, so the helper verifies the proof against
	// the value it actually sent.
	code := st.Code()
	writeMsg(t, conn, Message{Type: MsgPair, Code: code, Nonce: ch.Nonce})
	ok := readMsg(t, conn, 3*time.Second)
	if ok.Type != MsgPairOK || ok.Token == "" || ok.ServerKey == "" {
		t.Fatalf("want pair.ok with token and server key, got %+v", ok)
	}
	if want := PairProof(code, ch.Nonce, ok.Token, ok.ServerKey); ok.Proof != want {
		t.Fatalf("pair.ok proof does not verify against the pairing code")
	}
	return conn, ok.Token
}

func TestPairThenFetchRoundtrip(t *testing.T) {
	srv, st, wsURL := newTestServer(t)
	conn, token := handshake(t, wsURL, st)

	resCh := make(chan FetchResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := srv.Fetch(context.Background(), FetchRequest{
			URL: "https://example.com/private", Actions: []Action{{Type: "wait_selector", Selector: "#x"}},
			Recipe: "demo", Lane: "borrow", IO: "scripting",
		})
		resCh <- res
		errCh <- err
	}()

	frame := readMsg(t, conn, 3*time.Second)
	if frame.Type != MsgFetch || frame.URL != "https://example.com/private" || frame.Recipe != "demo" {
		t.Fatalf("unexpected fetch frame: %+v", frame)
	}
	if len(frame.Actions) != 1 || frame.Actions[0].Selector != "#x" {
		t.Fatalf("actions not forwarded: %+v", frame.Actions)
	}

	writeMsg(t, conn, Message{
		Type: MsgFetchResult, ID: frame.ID, Status: "ok",
		FinalURL: "https://example.com/private", Title: "Secret",
		Text: "secret text", HTML: "<html>secret</html>", IO: "scripting",
		HTTPStatus: 200, Markers: []string{},
	})

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Fetch error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fetch did not return")
	}
	res := <-resCh
	if res.Status != "ok" || res.Title != "Secret" || res.HTML != "<html>secret</html>" {
		t.Fatalf("bad result: %+v", res)
	}

	// The audit log carries one fetch line naming the recipe and origin.
	raw, err := os.ReadFile(filepath.Join(st.dir, "access.log.jsonl"))
	if err != nil {
		t.Fatalf("audit log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var last AuditEvent
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatalf("audit parse: %v", err)
	}
	if last.Event != "fetch" || last.Recipe != "demo" || last.Result != "ok" || last.Origin != "https://example.com" {
		t.Fatalf("bad audit line: %+v", last)
	}

	// A reconnect with the stored token is paired without a code, once the
	// server has proved itself with the key saved at pairing time.
	conn2 := dial(t, wsURL)
	ch2 := hello(t, conn2)
	writeMsg(t, conn2, Message{Type: MsgAuth, Token: token, Nonce: ch2.Nonce})
	if got := readMsg(t, conn2, 3*time.Second); got.Type != MsgStatus || !got.Paired {
		t.Fatalf("want paired status, got %+v", got)
	}
}

func TestFetchOffline(t *testing.T) {
	srv, _, _ := newTestServer(t)
	if _, err := srv.Fetch(context.Background(), FetchRequest{URL: "https://x.test/"}); !errors.Is(err, ErrOffline) {
		t.Fatalf("want ErrOffline, got %v", err)
	}
}

func TestFetchUnpaired(t *testing.T) {
	srv, _, wsURL := newTestServer(t)
	conn := dial(t, wsURL)
	_ = hello(t, conn)
	if _, err := srv.Fetch(context.Background(), FetchRequest{URL: "https://x.test/"}); !errors.Is(err, ErrUnpaired) {
		t.Fatalf("want ErrUnpaired, got %v", err)
	}
}

func TestFetchTimeoutSendsCancel(t *testing.T) {
	srv, st, wsURL := newTestServer(t)
	conn, _ := handshake(t, wsURL, st)

	go func() {
		_, _ = srv.Fetch(context.Background(), FetchRequest{URL: "https://slow.test/", TimeoutMS: 200})
	}()

	frame := readMsg(t, conn, 3*time.Second)
	if frame.Type != MsgFetch {
		t.Fatalf("want fetch, got %+v", frame)
	}
	// No reply. The server must give up and tell the extension to cancel.
	cancel := readMsg(t, conn, 3*time.Second)
	if cancel.Type != MsgCancel || cancel.ID != frame.ID {
		t.Fatalf("want cancel for %s, got %+v", frame.ID, cancel)
	}
}

func TestWrongPairCode(t *testing.T) {
	_, st, wsURL := newTestServer(t)
	conn := dial(t, wsURL)
	ch := hello(t, conn)
	writeMsg(t, conn, Message{Type: MsgPair, Code: "000000", Nonce: ch.Nonce})
	got := readMsg(t, conn, 3*time.Second)
	if got.Type != MsgError || got.Code != CodePairFailed {
		t.Fatalf("want pair-failed error, got %+v", got)
	}
	if st.ValidToken("anything") {
		t.Fatal("no token may exist after a failed pair")
	}
}

// TestPairStaleNonceRefused: a pair frame has to echo the nonce from this
// connection's challenge, so a recorded pairing cannot be replayed.
func TestPairStaleNonceRefused(t *testing.T) {
	_, st, wsURL := newTestServer(t)
	conn := dial(t, wsURL)
	_ = hello(t, conn)
	writeMsg(t, conn, Message{Type: MsgPair, Code: st.Code(), Nonce: "deadbeef"})
	got := readMsg(t, conn, 3*time.Second)
	if got.Type != MsgError || got.Code != CodeRefused {
		t.Fatalf("want refused error, got %+v", got)
	}
}

// TestPairBruteForceBurnsTheCode: the attempt counter lives in the state, not
// on the connection, so reconnecting cannot reset it. After MaxPairFails wrong
// guesses the code itself is replaced and even the right one is refused.
func TestPairBruteForceBurnsTheCode(t *testing.T) {
	_, st, wsURL := newTestServer(t)
	code := st.Code()
	st.mu.Lock()
	st.code = "111111" // deterministic wrong guesses
	st.mu.Unlock()

	for i := 0; i < MaxPairFails; i++ {
		conn := dial(t, wsURL)
		ch := hello(t, conn)
		writeMsg(t, conn, Message{Type: MsgPair, Code: "222222", Nonce: ch.Nonce})
		if got := readMsg(t, conn, 3*time.Second); got.Code != CodePairFailed {
			t.Fatalf("attempt %d: want pair-failed, got %+v", i, got)
		}
		conn.CloseNow()
	}

	// The old code is gone, and the attacker's reconnects do not help.
	conn := dial(t, wsURL)
	ch := hello(t, conn)
	writeMsg(t, conn, Message{Type: MsgPair, Code: code, Nonce: ch.Nonce})
	if got := readMsg(t, conn, 3*time.Second); got.Code != CodePairFailed {
		t.Fatalf("the burned code still paired: %+v", got)
	}
}

// TestOriginRefused: a web page in the user's own browser is a loopback peer,
// so its Origin must be refused even though the TCP peer check passes.
func TestOriginRefused(t *testing.T) {
	_, _, wsURL := newTestServer(t)
	for _, origin := range []string{"http://evil.test", "https://127.0.0.1:7431", "null"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
			HTTPHeader: http.Header{"Origin": []string{origin}},
		})
		cancel()
		if err == nil {
			conn.CloseNow()
			t.Fatalf("origin %q was accepted", origin)
		}
	}
}

// TestExtensionOriginAccepted: the extension's own origin is not http(s), and
// must get through the same check that refuses pages.
func TestExtensionOriginAccepted(t *testing.T) {
	_, _, wsURL := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"chrome-extension://abcdefghijklmnop"}},
	})
	if err != nil {
		t.Fatalf("extension origin refused: %v", err)
	}
	conn.CloseNow()
}

// TestUnpairRevokesLiveToken: revocation has to reach a connection that is
// already up. The socket deliberately stays OPEN here — closing it first would
// make the test pass on a server that never re-checks anything.
func TestUnpairRevokesLiveToken(t *testing.T) {
	srv, st, wsURL := newTestServer(t)
	conn, token := handshake(t, wsURL, st)

	// The connection works before revocation.
	before := make(chan error, 1)
	go func() {
		_, err := srv.Fetch(context.Background(), FetchRequest{URL: "https://x.test/", TimeoutMS: 300})
		before <- err
	}()
	frame := readMsg(t, conn, 3*time.Second)
	if frame.Type != MsgFetch {
		t.Fatalf("want a fetch, got %+v", frame)
	}
	writeMsg(t, conn, Message{Type: MsgFetchResult, ID: frame.ID, Status: "ok", HTML: "<html>hi</html>"})
	if err := <-before; err != nil {
		t.Fatalf("fetch before revocation: %v", err)
	}

	if n := st.Unpair("all"); n != 1 {
		t.Fatalf("Unpair removed %d devices, want 1", n)
	}
	if st.ValidToken(token) {
		t.Fatal("revoked token still validates")
	}
	// Same socket, still open: the next fetch must be refused.
	if _, err := srv.Fetch(context.Background(), FetchRequest{URL: "https://x.test/"}); !errors.Is(err, ErrUnpaired) {
		t.Fatalf("fetch after revocation: want ErrUnpaired, got %v", err)
	}
}

// TestPairLockoutIsPerCode: ten wrong guesses burn the code, and the code that
// replaces it starts a fresh budget. Without that reset the bridge would never
// pair again short of a restart.
func TestPairLockoutIsPerCode(t *testing.T) {
	_, st, wsURL := newTestServer(t)
	st.mu.Lock()
	st.code = "111111"
	st.mu.Unlock()

	for i := 0; i < MaxPairFails; i++ {
		conn := dial(t, wsURL)
		ch := hello(t, conn)
		writeMsg(t, conn, Message{Type: MsgPair, Code: "222222", Nonce: ch.Nonce})
		if got := readMsg(t, conn, 3*time.Second); got.Code != CodePairFailed {
			t.Fatalf("attempt %d: want pair-failed, got %+v", i, got)
		}
		conn.CloseNow()
	}

	fresh := st.Code()
	if fresh == "111111" {
		t.Fatal("the burned code was not replaced")
	}
	conn := dial(t, wsURL)
	ch := hello(t, conn)
	writeMsg(t, conn, Message{Type: MsgPair, Code: fresh, Nonce: ch.Nonce})
	got := readMsg(t, conn, 3*time.Second)
	if got.Type != MsgPairOK || got.Token == "" {
		t.Fatalf("the fresh code did not pair: %+v", got)
	}
}

// TestPairProofVerifiesWithTheTypedCode is the extension's side of the pairing
// handshake: it refuses a pair.ok whose proof does not match the code.
func TestPairProofVerifiesWithTheTypedCode(t *testing.T) {
	_, st, wsURL := newTestServer(t)
	code := st.Code()
	conn := dial(t, wsURL)
	ch := hello(t, conn)
	writeMsg(t, conn, Message{Type: MsgPair, Code: code, Nonce: ch.Nonce})
	ok := readMsg(t, conn, 3*time.Second)
	if ok.Type != MsgPairOK {
		t.Fatalf("want pair.ok, got %+v", ok)
	}
	if PairProof(code, ch.Nonce, ok.Token, ok.ServerKey) != ok.Proof {
		t.Fatal("honest server's proof failed its own check")
	}
	// A squatter on the port would have to produce this without knowing the
	// code: any other key is rejected.
	if PairProof("999999", ch.Nonce, ok.Token, ok.ServerKey) == ok.Proof {
		t.Fatal("a wrong code produced the same proof")
	}
}

func TestProtocolMismatchCloses(t *testing.T) {
	_, _, wsURL := newTestServer(t)
	conn := dial(t, wsURL)
	writeMsg(t, conn, Message{Type: MsgHello, Protocol: 99})
	got := readMsg(t, conn, 3*time.Second)
	if got.Type != MsgError || got.Code != CodeProtocolMismatch {
		t.Fatalf("want protocol-mismatch, got %+v", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatal("connection should be closed after protocol mismatch")
	}
}

func TestProxyHeaderRefused(t *testing.T) {
	_, _, wsURL := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"X-Forwarded-For": []string{"203.0.113.9"}},
	})
	if err == nil {
		conn.CloseNow()
		t.Fatal("dial with X-Forwarded-For should be refused")
	}
}

func TestRemoteIsLoopback(t *testing.T) {
	cases := []struct {
		remote string
		xff    string
		want   bool
	}{
		{"127.0.0.1:5000", "", true},
		{"[::1]:5000", "", true},
		{"10.0.0.5:5000", "", false},
		{"127.0.0.1:5000", "203.0.113.9", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/bridge", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := remoteIsLoopback(r); got != c.want {
			t.Errorf("remoteIsLoopback(%s, xff=%s) = %v, want %v", c.remote, c.xff, got, c.want)
		}
	}
}
