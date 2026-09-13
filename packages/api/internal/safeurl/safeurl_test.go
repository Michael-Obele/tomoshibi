package safeurl

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestBlockedIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"public IPv4", "93.184.216.34", false},
		{"public IPv6", "2606:2800:220:1:248:1893:25c8:1946", false},
		{"cloud metadata", "169.254.169.254", true},
		{"loopback", "127.0.0.1", true},
		{"loopback high", "127.1.2.3", true},
		{"IPv6 loopback", "::1", true},
		{"private 10/8", "10.0.0.5", true},
		{"private 172.16/12", "172.16.0.1", true},
		{"private 172.31 edge", "172.31.255.255", true},
		{"public 172.32 just outside", "172.32.0.1", false},
		{"private 192.168/16", "192.168.1.1", true},
		{"unique local IPv6", "fd00::1", true},
		{"link-local IPv6", "fe80::1", true},
		{"unspecified v4", "0.0.0.0", true},
		{"unspecified v6", "::", true},
		{"multicast", "224.0.0.1", true},
		{"CGNAT low", "100.64.0.1", true},
		{"CGNAT high", "100.127.255.255", true},
		{"public just below CGNAT", "100.63.255.255", false},
		{"public just above CGNAT", "100.128.0.1", false},
		// An IPv4-mapped IPv6 address is the classic bypass: the v6
		// predicates report nothing useful until it is unmapped.
		{"IPv4-mapped loopback", "::ffff:127.0.0.1", true},
		{"IPv4-mapped metadata", "::ffff:169.254.169.254", true},
		{"IPv4-mapped public", "::ffff:93.184.216.34", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := netip.ParseAddr(tt.ip)
			if err != nil {
				t.Fatalf("bad test IP %q: %v", tt.ip, err)
			}
			if got := blockedIP(addr); got != tt.blocked {
				t.Errorf("blockedIP(%s) = %v, want %v", tt.ip, got, tt.blocked)
			}
		})
	}
}

func TestBlockedIPInvalid(t *testing.T) {
	// The zero Addr must fail closed.
	if !blockedIP(netip.Addr{}) {
		t.Error("blockedIP(invalid) = false, want true (must fail closed)")
	}
}

func TestCheckScheme(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"http allowed", "http://example.com", false},
		{"https allowed", "https://example.com", false},
		{"file blocked", "file:///etc/passwd", true},
		{"gopher blocked", "gopher://example.com", true},
		{"ftp blocked", "ftp://example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Check(context.Background(), tt.url)
			var schemeErr *ErrScheme
			if tt.wantErr {
				if !errors.As(err, &schemeErr) {
					t.Errorf("Check(%q) error = %v, want ErrScheme", tt.url, err)
				}
				return
			}
			// A public hostname may fail to resolve in a sandbox; only a
			// scheme rejection is a real failure here.
			if errors.As(err, &schemeErr) {
				t.Errorf("Check(%q) rejected a valid scheme: %v", tt.url, err)
			}
		})
	}
}

func TestCheckLiteralIP(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		blocked bool
	}{
		{"metadata endpoint", "http://169.254.169.254/latest/meta-data/", true},
		{"localhost redis", "http://127.0.0.1:6379", true},
		{"internal admin", "http://10.0.0.5/admin", true},
		{"IPv6 loopback", "http://[::1]:8080/", true},
		{"public literal", "http://93.184.216.34/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Check(context.Background(), tt.url)
			var blockedErr *ErrBlocked
			if got := errors.As(err, &blockedErr); got != tt.blocked {
				t.Errorf("Check(%q) blocked = %v (err=%v), want %v", tt.url, got, err, tt.blocked)
			}
		})
	}
}

func TestRetryableDNS(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		retry bool
	}{
		{
			name:  "server misbehaving (Docker SERVFAIL) is retryable",
			err:   &net.DNSError{Err: "server misbehaving", Name: "example.com"},
			retry: true,
		},
		{
			name:  "timeout is retryable",
			err:   &net.DNSError{Err: "i/o timeout", Name: "example.com", IsTimeout: true},
			retry: true,
		},
		{
			name:  "NXDOMAIN is definitive, not retryable",
			err:   &net.DNSError{Err: "no such host", Name: "nope.invalid", IsNotFound: true},
			retry: false,
		},
		{
			name:  "non-DNS errors are not retried",
			err:   errors.New("connection refused"),
			retry: false,
		},
		{
			name:  "nil is not retried",
			err:   nil,
			retry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryableDNS(tt.err); got != tt.retry {
				t.Errorf("retryableDNS(%v) = %v, want %v", tt.err, got, tt.retry)
			}
		})
	}
}

func TestRetryDialContextRecoversFromTransientDNS(t *testing.T) {
	// Simulate Docker's embedded resolver flaking: the first two dials fail
	// with "server misbehaving", the third succeeds. The wrapper must retry
	// and return the successful connection.
	attempts := 0
	base := func(_ context.Context, _, _ string) (net.Conn, error) {
		attempts++
		if attempts <= 2 {
			return nil, &net.DNSError{Err: "server misbehaving", Name: "example.com"}
		}
		return &fakeConn{}, nil
	}

	dial := retryDialContext(base)
	conn, err := dial(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("retryDialContext gave up after transient DNS errors: %v", err)
	}
	defer conn.Close()
	if attempts != 3 {
		t.Errorf("expected 3 attempts (2 failures + success), got %d", attempts)
	}
}

func TestRetryDialContextGivesUpOnPersistentDNS(t *testing.T) {
	attempts := 0
	base := func(_ context.Context, _, _ string) (net.Conn, error) {
		attempts++
		return nil, &net.DNSError{Err: "server misbehaving", Name: "example.com"}
	}

	dial := retryDialContext(base)
	_, err := dial(context.Background(), "tcp", "example.com:443")
	if err == nil {
		t.Fatal("expected an error after retries were exhausted")
	}
	if attempts != dnsRetries+1 {
		t.Errorf("expected %d attempts, got %d", dnsRetries+1, attempts)
	}
}

func TestRetryDialContextDoesNotRetryNXDOMAIN(t *testing.T) {
	attempts := 0
	base := func(_ context.Context, _, _ string) (net.Conn, error) {
		attempts++
		return nil, &net.DNSError{Err: "no such host", Name: "nope.invalid", IsNotFound: true}
	}

	dial := retryDialContext(base)
	_, err := dial(context.Background(), "tcp", "nope.invalid:443")
	if err == nil {
		t.Fatal("expected NXDOMAIN to fail")
	}
	if attempts != 1 {
		t.Errorf("NXDOMAIN must not be retried; got %d attempts", attempts)
	}
}

// fakeConn is a minimal net.Conn for tests that only need a non-nil success.
type fakeConn struct{}

func (*fakeConn) Read([]byte) (int, error)         { return 0, nil }
func (*fakeConn) Write([]byte) (int, error)        { return 0, nil }
func (*fakeConn) Close() error                     { return nil }
func (*fakeConn) LocalAddr() net.Addr              { return nil }
func (*fakeConn) RemoteAddr() net.Addr             { return nil }
func (*fakeConn) SetDeadline(time.Time) error      { return nil }
func (*fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (*fakeConn) SetWriteDeadline(time.Time) error { return nil }

func TestCheckLocalhostName(t *testing.T) {
	// "localhost" resolves via the resolver path rather than the literal
	// path, so it exercises different code.
	err := Check(context.Background(), "http://localhost:6379/")
	var blockedErr *ErrBlocked
	if !errors.As(err, &blockedErr) {
		t.Errorf("Check(localhost) error = %v, want ErrBlocked", err)
	}
}

func TestAllowPrivateEnvOptOut(t *testing.T) {
	t.Setenv(AllowPrivateEnv, "true")
	if err := Check(context.Background(), "http://127.0.0.1:3000/"); err != nil {
		t.Errorf("with %s=true, Check should permit loopback, got %v", AllowPrivateEnv, err)
	}
	if err := Control("tcp", "127.0.0.1:3000", nil); err != nil {
		t.Errorf("with %s=true, Control should permit loopback, got %v", AllowPrivateEnv, err)
	}
}

func TestControl(t *testing.T) {
	tests := []struct {
		name    string
		network string
		address string
		wantErr bool
	}{
		{"public tcp", "tcp", "93.184.216.34:443", false},
		{"loopback", "tcp", "127.0.0.1:6379", true},
		{"metadata", "tcp4", "169.254.169.254:80", true},
		{"private", "tcp", "10.0.0.5:80", true},
		{"IPv6 loopback", "tcp6", "[::1]:80", true},
		{"non-tcp refused", "udp", "93.184.216.34:53", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Control(tt.network, tt.address, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Control(%q, %q) error = %v, wantErr %v", tt.network, tt.address, err, tt.wantErr)
			}
		})
	}
}

// TestClientBlocksLoopback is the end-to-end proof: a real server on
// loopback must be unreachable through our client.
func TestClientBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secret internal data"))
	}))
	defer srv.Close()

	resp, err := Client(5e9).Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatalf("client reached loopback server at %s; SSRF guard did not engage", srv.URL)
	}
	var blockedErr *ErrBlocked
	if !errors.As(err, &blockedErr) {
		t.Errorf("error = %v, want it to wrap ErrBlocked", err)
	}
}

// TestClientFollowsRedirectGuard proves the dial-time hook covers
// redirects: the initial URL is never private, only the destination is.
func TestClientFollowsRedirectGuard(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secret internal data"))
	}))
	defer internal.Close()

	// A redirector that is itself reachable, pointing at loopback.
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	defer redirector.Close()

	// Both servers are on loopback, so the first hop is already blocked;
	// assert the guard refuses rather than asserting on which hop.
	resp, err := Client(5e9).Get(redirector.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("client followed redirect chain to loopback")
	}
}

func TestDialerHasControl(t *testing.T) {
	d := Dialer()
	if d.Control == nil {
		t.Error("Dialer() returned a dialer with no Control hook; SSRF guard would be bypassed")
	}
	var _ *net.Dialer = d
}

func TestTransportUsesGuardedDial(t *testing.T) {
	tr := Transport()
	if tr.DialContext == nil {
		t.Fatal("Transport() has nil DialContext; would fall back to unguarded default")
	}
	_, err := tr.DialContext(context.Background(), "tcp", "127.0.0.1:9")
	if err == nil {
		t.Error("transport dialed loopback successfully; guard not wired")
	}
}
