package bridge

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State persists pairing material under the state dir:
//
//	~/.config/tomoshi/bridge.json        pairing code, port, device hashes,
//	                                     server key
//	~/.config/tomoshi/access.log.jsonl   one audit line per event
//
// Tokens are stored as sha256 hashes only; the plaintext token exists exactly
// once, in the pair.ok reply the extension saves. The server key is stored as
// itself: it proves the server's identity to the extension during the
// handshake, and it grants nothing on its own (a reader of this file already
// has every token hash's worth of local access).
type State struct {
	mu        sync.Mutex
	dir       string
	path      string
	logger    *slog.Logger
	code      string
	codeAt    time.Time
	port      string
	serverKey string
	devices   []Device
	// pairFails counts failed pairing attempts since the current code was
	// issued. It lives here, not on the connection, because a client that can
	// reconnect can otherwise reset a per-connection counter and walk the
	// whole 10^6 space inside one code lifetime.
	pairFails int
	// mtime of the file we last wrote, to notice an external edit (a CLI
	// unpair, or the documented "delete bridge.json to revoke").
	mtime time.Time
}

// Device is one paired browser (token hash + label).
type Device struct {
	Name      string    `json:"name"`
	TokenSHA  string    `json:"token_sha256"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
}

type stateFile struct {
	Port      string   `json:"port,omitempty"`
	Code      string   `json:"code,omitempty"`
	CodeAt    string   `json:"code_at,omitempty"`
	ServerKey string   `json:"server_key,omitempty"`
	Devices   []Device `json:"devices,omitempty"`
}

// CodeTTL is how long a pairing code stays valid; the server rotates the code
// on this interval and after every successful pair.
const CodeTTL = 10 * time.Minute

// MaxPairFails is how many wrong codes are tolerated inside one code's
// lifetime. After that the code is invalidated and the user has to ask for a
// fresh one, which caps a brute-force sweep at a handful of guesses per code.
const MaxPairFails = 10

func NewState(dir string, logger *slog.Logger) (*State, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if dir == "" {
		return nil, errors.New("bridge: empty state dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("bridge: state dir: %w", err)
	}
	s := &State{dir: dir, path: filepath.Join(dir, "bridge.json"), logger: logger}
	if raw, err := os.ReadFile(s.path); err == nil {
		var f stateFile
		if err := json.Unmarshal(raw, &f); err == nil {
			s.port = f.Port
			s.code = f.Code
			s.serverKey = f.ServerKey
			s.devices = f.Devices
			if t, err := time.Parse(time.RFC3339, f.CodeAt); err == nil {
				s.codeAt = t
			}
		}
		if st, err := os.Stat(s.path); err == nil {
			s.mtime = st.ModTime()
		}
	}
	if s.serverKey == "" {
		s.serverKey = randomHex(32)
	}
	if s.code == "" || time.Since(s.codeAt) > CodeTTL {
		s.rotateLocked()
		// Persist immediately: the CLI and the extension popup read this file,
		// and a code that only exists in memory is a code nobody can enter.
		if err := s.saveLocked(); err != nil {
			return nil, fmt.Errorf("bridge: write state: %w", err)
		}
	}
	return s, nil
}

// DefaultStateDir is the state directory both the API and the CLI use when no
// override is given.
func DefaultStateDir() string {
	if dir := os.Getenv("BRIDGE_STATE_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "tomoshi")
}

// Info is the read-only view of bridge state that the CLI prints. It never
// rotates anything, so reading it can never invalidate a code the running
// server is showing.
type Info struct {
	Port      string    `json:"port,omitempty"`
	Code      string    `json:"code,omitempty"`
	CodeAt    time.Time `json:"code_at,omitempty"`
	Devices   []Device  `json:"devices,omitempty"`
	StatePath string    `json:"state_path,omitempty"`
}

// ReadInfo reads the state file without side effects.
func ReadInfo(dir string) (Info, error) {
	path := filepath.Join(dir, "bridge.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	var f stateFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return Info{}, fmt.Errorf("bridge: state file unreadable: %w", err)
	}
	info := Info{Port: f.Port, Code: f.Code, Devices: f.Devices, StatePath: path}
	if t, err := time.Parse(time.RFC3339, f.CodeAt); err == nil {
		info.CodeAt = t
	}
	return info, nil
}

// Code returns the current pairing code, rotating it first when stale.
func (s *State) Code() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.codeAt) > CodeTTL {
		s.rotateLocked()
		s.saveLocked()
	}
	return s.code
}

// Publish records the port the API is listening on so the CLI can report it.
func (s *State) Publish(port string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.port = port
	return s.saveLocked()
}

// Port returns the last published port ("" if never published).
func (s *State) Port() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// Devices returns a copy of the paired-device list.
func (s *State) Devices() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Device, len(s.devices))
	copy(out, s.devices)
	return out
}

// Pair validates a code and returns a fresh device token and the server key
// the extension must keep to verify future handshakes. The code rotates on
// success, so a code works exactly once.
func (s *State) Pair(code, name string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloadIfChangedLocked()
	if s.pairFails >= MaxPairFails {
		return "", "", errors.New("too many wrong codes; run `tomoshibi bridge code` for a new one")
	}
	if time.Since(s.codeAt) > CodeTTL || s.code == "" {
		return "", "", errors.New("pairing code expired; read the current code from the server log or `tomoshibi bridge code`")
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(s.code)) != 1 {
		// Burn the code after a handful of misses so a reconnect loop cannot
		// walk the 10^6 space.
		s.pairFails++
		if s.pairFails >= MaxPairFails {
			s.rotateLocked()
			if err := s.saveLocked(); err != nil {
				s.logger.Warn("bridge: state save failed", "error", err)
			}
		}
		return "", "", errors.New("pairing code does not match")
	}
	tok := randomHex(32)
	s.devices = append(s.devices, Device{
		Name:      name,
		TokenSHA:  hashToken(tok),
		CreatedAt: time.Now().UTC(),
	})
	s.pairFails = 0
	s.rotateLocked()
	if err := s.saveLocked(); err != nil {
		return "", "", err
	}
	return tok, s.serverKey, nil
}

// ServerKey returns the key the extension verifies the server's handshake
// proof with. It is written into the state file so a restart keeps it.
func (s *State) ServerKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloadIfChangedLocked()
	return s.serverKey
}

// Unpair removes every paired device (or one named device) and persists it.
func (s *State) Unpair(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloadIfChangedLocked()
	before := len(s.devices)
	if name == "" || name == "all" {
		s.devices = nil
	} else {
		kept := s.devices[:0]
		for _, d := range s.devices {
			if d.Name != name {
				kept = append(kept, d)
			}
		}
		s.devices = kept
	}
	if err := s.saveLocked(); err != nil {
		s.logger.Warn("bridge: state save failed", "error", err)
	}
	return before - len(s.devices)
}

// ValidToken reports whether tok belongs to a paired device, and refreshes
// that device's LastSeen. It also picks up an external edit of the state file,
// so `tomoshibi bridge unpair` (or deleting bridge.json) takes effect on a
// running server instead of being silently rewritten back on the next rotate.
func (s *State) ValidToken(tok string) bool {
	if tok == "" {
		return false
	}
	h := hashToken(tok)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloadIfChangedLocked()
	for i := range s.devices {
		if subtle.ConstantTimeCompare([]byte(h), []byte(s.devices[i].TokenSHA)) == 1 {
			s.devices[i].LastSeen = time.Now().UTC()
			return true
		}
	}
	return false
}

// HasDevice reports whether a token hash still belongs to a paired device. It
// is the check a live connection has to pass on every fetch, so unpairing (or
// deleting the state file) revokes a browser that is already connected.
func (s *State) HasDevice(tokenSHA string) bool {
	if tokenSHA == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloadIfChangedLocked()
	for i := range s.devices {
		if subtle.ConstantTimeCompare([]byte(tokenSHA), []byte(s.devices[i].TokenSHA)) == 1 {
			return true
		}
	}
	return false
}

// DeviceTokenSHA returns the hash for a presented token, so a connection can
// remember which device it authenticated as.
func DeviceTokenSHA(tok string) string { return hashToken(tok) }

// reloadIfChangedLocked re-reads the state file when something else wrote it
// (the CLI, or a user deleting it to revoke). Callers hold s.mu.
func (s *State) reloadIfChangedLocked() {
	st, err := os.Stat(s.path)
	if err != nil {
		// Deleted: treat every device as revoked.
		if os.IsNotExist(err) && len(s.devices) > 0 {
			s.devices = nil
		}
		return
	}
	if !st.ModTime().After(s.mtime) {
		return
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var f stateFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return
	}
	s.mtime = st.ModTime()
	s.devices = f.Devices
	if f.ServerKey != "" {
		s.serverKey = f.ServerKey
	}
	if f.Code != "" && f.Code != s.code {
		// Someone else issued a code (the CLI, or a rotation we did not do):
		// the attempt budget belongs to a code, so it starts over with it.
		s.pairFails = 0
	}
	if f.Code != "" {
		s.code = f.Code
		if t, err := time.Parse(time.RFC3339, f.CodeAt); err == nil {
			s.codeAt = t
		}
	}
}

// Audit appends one JSONL line. Audit failures are logged, never fatal: a run
// must not fail because the log disk is full.
func (s *State) Audit(ev AuditEvent) {
	if ev.TS == "" {
		ev.TS = nowRFC3339()
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(s.dir, "access.log.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		s.logger.Warn("bridge: audit open failed", "error", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		s.logger.Warn("bridge: audit write failed", "error", err)
	}
}

func (s *State) rotateLocked() {
	s.code = randomCode()
	s.codeAt = time.Now().UTC()
	// A fresh code is a fresh budget. Without this the counter burned through
	// one code would refuse every later code too, and the only way back would be
	// a restart.
	s.pairFails = 0
}

func (s *State) saveLocked() error {
	f := stateFile{
		Port: s.port, Code: s.code, CodeAt: s.codeAt.Format(time.RFC3339),
		ServerKey: s.serverKey, Devices: s.devices,
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	if st, err := os.Stat(s.path); err == nil {
		s.mtime = st.ModTime()
	}
	return nil
}

func randomCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		// crypto/rand failing is already catastrophic on Linux, and pairing
		// still needs *a* code; the time-based fallback only has to rotate.
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// HMAC helpers for the handshake proof. A caller cannot verify a peer without
// these, so they live with the state that owns the key.

// ServerProof is what the server signs in a challenge: HMAC(server_key, nonce).
func ServerProof(serverKey, nonce string) string {
	mac := hmac.New(sha256.New, []byte(serverKey))
	mac.Write([]byte("tomoshibi-srv|" + nonce))
	return hex.EncodeToString(mac.Sum(nil))
}

// PairProof is what the server signs in pair.ok: the extension verifies it
// with the pairing code the user typed, so a squatter on the port cannot
// pretend a pairing succeeded. Binding the token and the server key makes the
// proof unusable anywhere else.
func PairProof(code, nonce, token, serverKey string) string {
	mac := hmac.New(sha256.New, []byte(code))
	mac.Write([]byte("tomoshibi-pair|" + nonce + "|" + token + "|" + serverKey))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(time.Now().String())))[:n*2]
	}
	return hex.EncodeToString(b)
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}
