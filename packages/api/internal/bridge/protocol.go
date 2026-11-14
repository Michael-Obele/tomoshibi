// Package bridge implements the browser bridge: a loopback WebSocket that lets
// a paired browser extension fetch and drive pages inside the user's own
// signed-in browser, and hand the result back to the scraper.
//
// Security posture: loopback-only listeners, an origin allow-list on the
// upgrade (a web page in the user's own browser is a loopback peer), and a
// mutual-auth handshake so that neither side releases a secret to an
// unverified peer:
//
//	extension                          server
//	hello ──────────────────────────────▶
//	     ◀──────────── challenge {nonce, proof=HMAC(server_key,nonce)}
//	   (verify proof with the server_key saved at pairing; a squatter on the
//	    port cannot produce it, so the token is never released)
//	auth {token, nonce} ────────────────▶
//	     ◀──────────── status {paired:true}
//
// Unpaired clients instead get pair.required {nonce} and answer with
// pair {code, nonce}; the reply carries the new server_key plus a proof keyed
// by the pairing code the user just typed, so a fake server cannot hand the
// extension a token it cannot verify.
package bridge

import (
	"errors"
	"time"
)

// ProtocolVersion is the wire version. The extension pins it and refuses a
// mismatch; bump it on any breaking change to Message.
const ProtocolVersion = 1

// MaxMessageBytes caps one inbound frame. The numbers have to compose: the
// extension caps HTML at 8 MB and text at 2 MB, and a full-page PNG arrives as
// base64 (four bytes per three, plus JSON escaping). 24 MiB leaves that
// combination room, while still refusing a frame that would let one fetch
// exhaust a small host (multi-scrape runs 5 fetches concurrently). A frame over
// the limit closes the socket, so the extension also refuses to send an
// oversized screenshot and says so instead.
const MaxMessageBytes = 24 << 20

// Message types.
const (
	MsgHello        = "hello"
	MsgChallenge    = "challenge"
	MsgAuth         = "auth"
	MsgPairRequired = "pair.required"
	MsgPair         = "pair"
	MsgPairOK       = "pair.ok"
	MsgFetch        = "fetch"
	MsgFetchResult  = "fetch.result"
	MsgCancel       = "cancel"
	MsgStatus       = "status"
	MsgPing         = "ping"
	MsgPong         = "pong"
	MsgError        = "error"
)

// Error codes carried in Message.Code.
const (
	CodeProtocolMismatch = "protocol-mismatch"
	CodePairFailed       = "pair-failed"
	CodePairLocked       = "pair-locked"
	CodeRefused          = "refused"
)

// Fetcher errors surfaced to the scraper. They are deliberately user-facing:
// the message is what an agent author sees when the bridge is not ready.
var (
	ErrOffline  = errors.New("bridge: no browser connected (install the bridge extension and pair it)")
	ErrUnpaired = errors.New("bridge: the connected browser is not paired")
	ErrTimeout  = errors.New("bridge: the browser did not answer in time")
	// ErrDisconnected means the socket went away mid-fetch, which is not the
	// same as a slow page: the caller should retry rather than raise a timeout.
	ErrDisconnected = errors.New("bridge: the browser disconnected mid-fetch")
)

// Action is one page interaction the extension performs, in order, inside the
// pinned tab before the page is read back.
type Action struct {
	Type     string `json:"type"` // wait_selector|wait_ms|scroll_to_bottom|scroll_by|click|type|press|navigate|evaluate|wait_for_function
	Selector string `json:"selector,omitempty"`
	Text     string `json:"text,omitempty"`
	Key      string `json:"key,omitempty"`
	URL      string `json:"url,omitempty"`
	Script   string `json:"script,omitempty"`
	Ms       int    `json:"ms,omitempty"`
	Px       int    `json:"px,omitempty"`
}

// ActionResult is the extension's report for one action.
type ActionResult struct {
	Type  string `json:"type"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// FetchRequest asks the browser to load a URL, run actions, and read it back.
type FetchRequest struct {
	URL          string   `json:"url"`
	Actions      []Action `json:"actions,omitempty"`
	WaitSelector string   `json:"wait_selector,omitempty"`
	SettleMS     int      `json:"settle_ms,omitempty"`
	TimeoutMS    int      `json:"timeout_ms,omitempty"`
	Screenshot   bool     `json:"screenshot,omitempty"`
	FullPage     bool     `json:"full_page,omitempty"`
	IO           string   `json:"io,omitempty"` // scripting (default) | turbo

	// Recipe metadata travels for the audit log only; the extension does not
	// interpret it, and the API marks the page as untrusted in the response.
	Recipe string `json:"recipe,omitempty"`
	Lane   string `json:"lane,omitempty"`
}

// FetchResult is what the browser observed.
type FetchResult struct {
	// Status is ok, awaiting-human, blocked, or error.
	Status        string         `json:"status"`
	FinalURL      string         `json:"final_url,omitempty"`
	Title         string         `json:"title,omitempty"`
	Text          string         `json:"text,omitempty"`
	HTML          string         `json:"html,omitempty"`
	HTMLTruncated bool           `json:"html_truncated,omitempty"`
	ScreenshotB64 string         `json:"screenshot_b64,omitempty"`
	Markers       []string       `json:"markers,omitempty"`
	ActionsRun    []ActionResult `json:"actions_run,omitempty"`
	IO            string         `json:"io,omitempty"`
	HTTPStatus    int            `json:"http_status,omitempty"`
	Error         string         `json:"error,omitempty"`
	Ms            int64          `json:"ms,omitempty"`
}

// Message is the single envelope for every frame in both directions. One
// struct with omitempty keeps the protocol debuggable with a plain JSON dump.
type Message struct {
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`
	Protocol int    `json:"protocol,omitempty"`

	// hello / pair / auth
	Browser string `json:"browser,omitempty"`
	Version string `json:"version,omitempty"`
	Token   string `json:"token,omitempty"`
	// Code is the pairing code in a pair frame and the error code in an error
	// frame; Type disambiguates.
	Code string `json:"code,omitempty"`
	// Nonce and Proof carry the mutual-auth handshake (see the package doc).
	Nonce     string `json:"nonce,omitempty"`
	Proof     string `json:"proof,omitempty"`
	ServerKey string `json:"server_key,omitempty"`

	// status
	Paired   bool `json:"paired,omitempty"`
	Paused   bool `json:"paused,omitempty"`
	TabCount int  `json:"tab_count,omitempty"`

	// fetch
	URL          string   `json:"url,omitempty"`
	Actions      []Action `json:"actions,omitempty"`
	WaitSelector string   `json:"wait_selector,omitempty"`
	SettleMS     int      `json:"settle_ms,omitempty"`
	TimeoutMS    int      `json:"timeout_ms,omitempty"`
	Screenshot   bool     `json:"screenshot,omitempty"`
	FullPage     bool     `json:"full_page,omitempty"`
	IO           string   `json:"io,omitempty"`
	Recipe       string   `json:"recipe,omitempty"`
	Lane         string   `json:"lane,omitempty"`

	// fetch.result
	Status        string         `json:"status,omitempty"`
	FinalURL      string         `json:"final_url,omitempty"`
	Title         string         `json:"title,omitempty"`
	Text          string         `json:"text,omitempty"`
	HTML          string         `json:"html,omitempty"`
	HTMLTruncated bool           `json:"html_truncated,omitempty"`
	ScreenshotB64 string         `json:"screenshot_b64,omitempty"`
	Markers       []string       `json:"markers,omitempty"`
	ActionsRun    []ActionResult `json:"actions_run,omitempty"`
	HTTPStatus    int            `json:"http_status,omitempty"`
	Ms            int64          `json:"ms,omitempty"`

	// Message is the human-readable detail on an error frame.
	Message string `json:"message,omitempty"`
}

// AuditEvent is one line in access.log.jsonl: what was read, as whom, when.
type AuditEvent struct {
	TS      string `json:"ts"`
	Event   string `json:"event"` // pair | fetch | refuse
	Device  string `json:"device,omitempty"`
	Recipe  string `json:"recipe,omitempty"`
	Lane    string `json:"lane,omitempty"`
	IO      string `json:"io,omitempty"`
	Origin  string `json:"origin,omitempty"`
	Actions int    `json:"actions,omitempty"`
	// Pages is how many pages the run read. The bridge fetches one at a time,
	// so it is 1 today; the field exists because the log is the record of what
	// was read, and a page budget has to be countable from it.
	Pages  int    `json:"pages,omitempty"`
	Result string `json:"result,omitempty"`
	Ms     int64  `json:"ms,omitempty"`
	Note   string `json:"note,omitempty"`
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
