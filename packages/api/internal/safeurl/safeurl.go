// Package safeurl guards outbound fetches against SSRF.
//
// Cinder's entire job is to fetch a URL the caller names and hand back the
// body, so an unguarded instance is a proxy into whatever network it runs
// on: cloud metadata endpoints (169.254.169.254), Redis on localhost, admin
// panels on 10.0.0.0/8. Auth is off by default, so a public deploy is
// reachable by anyone.
//
// Two layers of defense, because either alone is insufficient:
//
//   - Check validates a URL up front and is used where the fetch is not
//     performed by our own http.Client (chromedp drives a real browser).
//   - Control is a net.Dialer hook that runs after DNS resolution with the
//     address actually being connected to. It therefore covers redirects
//     and DNS rebinding, which a pre-flight check on the original URL
//     cannot: a host that resolves to a public IP when validated and a
//     private one microseconds later at dial time is a real attack, and
//     only the dial-time check sees it.
package safeurl

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"syscall"
	"time"
)

// AllowPrivateEnv opts out of private-address blocking, for operators who
// legitimately scrape an internal wiki or a local dev server.
const AllowPrivateEnv = "SSRF_ALLOW_PRIVATE"

// dnsRetries is how many times a transient DNS failure is retried before
// giving up. Docker's embedded resolver (127.0.0.11) intermittently returns
// SERVFAIL / "server misbehaving" under burst load — a quick retry usually
// succeeds and turns a whole scrape failure into a blip.
const dnsRetries = 3

// retryableDNS reports whether a lookup/dial error is worth retrying.
// NXDOMAIN ("no such host") is definitive — retrying cannot help. Everything
// else from the resolver (timeouts, SERVFAIL, "server misbehaving", refused)
// is transient and may succeed on a retry.
func retryableDNS(err error) bool {
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		return false
	}
	return !dnsErr.IsNotFound
}

// dnsBackoff returns the wait before retry attempt n (0-based).
func dnsBackoff(attempt int) time.Duration {
	return time.Duration(attempt+1) * 250 * time.Millisecond
}

// resolveWithRetry resolves host, retrying transient DNS failures with a
// short backoff. It returns the first successful answer set, or the last
// error once retries are exhausted.
func resolveWithRetry(ctx context.Context, host string) ([]netip.Addr, error) {
	var lastErr error
	for attempt := 0; attempt <= dnsRetries; attempt++ {
		addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err == nil {
			return addrs, nil
		}
		lastErr = err
		if !retryableDNS(err) {
			return nil, err
		}
		if attempt < dnsRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(dnsBackoff(attempt)):
			}
		}
	}
	return nil, lastErr
}

// ErrBlocked is returned when a destination resolves to a blocked address.
type ErrBlocked struct {
	Host string
	IP   string
}

func (e *ErrBlocked) Error() string {
	if e.IP != "" && e.IP != e.Host {
		return fmt.Sprintf("blocked request to non-public address %q (%q)", e.Host, e.IP)
	}
	return fmt.Sprintf("blocked request to non-public address %q", e.Host)
}

// ErrScheme is returned for non-HTTP(S) URLs. file:// and gopher:// are
// classic SSRF escalation vectors, so the scheme is allowlisted.
type ErrScheme struct{ Scheme string }

func (e *ErrScheme) Error() string {
	return fmt.Sprintf("unsupported URL scheme %q (only http and https are allowed)", e.Scheme)
}

// allowPrivate reports whether private-address blocking is disabled. It is
// read at call time rather than cached so tests and running processes can
// flip it without a restart, matching the env handling in crawl_handler.go.
func allowPrivate() bool {
	v := os.Getenv(AllowPrivateEnv)
	return v == "true" || v == "1"
}

// blockedIP reports whether addr is one we refuse to connect to.
//
// The check is an allowlist in spirit: anything that is not a routable
// public unicast address is refused, so a range nobody thought of fails
// closed rather than open.
func blockedIP(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}

	// An IPv4-mapped IPv6 address (::ffff:127.0.0.1) carries an IPv4 value
	// whose properties the IPv6 methods do not report. Unwrap so the checks
	// below see the address the kernel will actually route to.
	addr = addr.Unmap()

	// 127.0.0.0/8 and ::1; 10/8, 172.16/12, 192.168/16, fc00::/7;
	// 169.254/16 (cloud metadata) and fe80::/10; every multicast scope; and
	// the unspecified address, which routes to localhost on Linux.
	switch {
	case addr.IsLoopback(),
		addr.IsPrivate(),
		addr.IsLinkLocalUnicast(),
		addr.IsLinkLocalMulticast(),
		addr.IsInterfaceLocalMulticast(),
		addr.IsMulticast(),
		addr.IsUnspecified():
		return true
	}

	// Carrier-grade NAT (100.64.0.0/10). Not "private" per Go's definition,
	// but several cloud providers expose internal services there.
	if addr.Is4() {
		b := addr.As4()
		if b[0] == 100 && b[1] >= 64 && b[1] <= 127 {
			return true
		}
	}

	return false
}

// Check validates a URL before a fetch: the scheme must be http(s), and
// every address the host resolves to must be public.
//
// Callers that fetch through Client or Transport get a stronger guarantee
// from Control and do not need this. Use it where the actual connection is
// made by something we do not control, such as a browser.
func Check(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return &ErrScheme{Scheme: u.Scheme}
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL has no host")
	}
	if allowPrivate() {
		return nil
	}

	// A literal IP needs no resolution.
	if addr, err := netip.ParseAddr(host); err == nil {
		if blockedIP(addr) {
			return &ErrBlocked{Host: host, IP: addr.String()}
		}
		return nil
	}

	addrs, err := resolveWithRetry(ctx, host)
	if err != nil {
		return fmt.Errorf("could not resolve %s: %w", host, err)
	}
	// Every answer must be public. A hostname with one public and one
	// private A record would otherwise be exploitable by retrying until
	// the resolver returns the private one.
	for _, addr := range addrs {
		if blockedIP(addr) {
			return &ErrBlocked{Host: host, IP: addr.String()}
		}
	}
	return nil
}

// Control is a net.Dialer Control hook that refuses connections to
// non-public addresses. It runs after DNS resolution on the address being
// dialed, so it covers redirect chains and DNS rebinding alike.
func Control(network, address string, _ syscall.RawConn) error {
	if allowPrivate() {
		return nil
	}
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return fmt.Errorf("blocked non-TCP network %q", network)
	}

	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("could not parse dial address %q: %w", address, err)
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		// The dialer hands us a resolved literal; anything else is
		// unexpected and we fail closed.
		return fmt.Errorf("could not parse dial IP %q: %w", host, err)
	}
	if blockedIP(addr) {
		return &ErrBlocked{Host: host, IP: addr.String()}
	}
	return nil
}

// Dialer returns a net.Dialer that refuses non-public destinations.
func Dialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   Control,
	}
}

// retryDialContext wraps a dial function so that transient DNS failures are
// retried with a short backoff before giving up.
//
// The retry matters in containerized deployments: Docker's embedded resolver
// (127.0.0.11) intermittently answers SERVFAIL / "server misbehaving" under
// burst load. Without a retry, a single bad answer fails the whole scrape.
// NXDOMAIN is not retried — the host genuinely does not exist.
func retryDialContext(
	base func(ctx context.Context, network, addr string) (net.Conn, error),
) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		var lastErr error
		for attempt := 0; attempt <= dnsRetries; attempt++ {
			conn, err := base(ctx, network, addr)
			if err == nil {
				return conn, nil
			}
			lastErr = err
			if !retryableDNS(err) {
				return nil, err
			}
			if attempt < dnsRetries {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(dnsBackoff(attempt)):
				}
			}
		}
		return nil, lastErr
	}
}

// Transport returns an http.Transport that refuses non-public destinations
// and retries transient DNS failures.
func Transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = retryDialContext(Dialer().DialContext)
	return t
}

// Client returns an http.Client that refuses non-public destinations.
func Client(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: Transport(),
	}
}
