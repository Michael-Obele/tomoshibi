// Package fingerprint gives outbound scraper requests a coherent browser
// identity: a Chrome TLS ClientHello, Chrome's HTTP/2 settings and header
// order (via bogdanfinn/tls-client), and a User-Agent that matches the
// profile.
//
// The coherence is the point. A random Chrome User-Agent sent over Go's
// default TLS stack is a louder bot signal than one honest identity, because
// the UA and the handshake disagree. tls-client is already a direct
// dependency (the search engine layer uses it for the same reason), so this
// costs no new module.
package fingerprint

import (
	"fmt"
	"io"
	"net/http"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	"github.com/Michael-Obele/tomoshibi/internal/safeurl"
)

// ChromeUA is the User-Agent that matches profiles.DefaultClientProfile
// (Chrome_150). Keep the two in step: change one without the other and the
// request becomes internally inconsistent, which is the exact failure this
// package exists to prevent.
const ChromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"

// DefaultHeaders are the navigation headers a real Chrome sends for a
// top-level document request. A bare User-Agent is itself a fingerprint.
func DefaultHeaders() map[string]string {
	return map[string]string{
		"User-Agent":                ChromeUA,
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
		"Accept-Language":           "en-US,en;q=0.9",
		"Upgrade-Insecure-Requests": "1",
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
		"Sec-Fetch-User":            "?1",
	}
}

// Transport returns a RoundTripper that performs HTTPS requests with Chrome's
// TLS fingerprint. Redirects are left to net/http so the caller's policy
// (Colly's redirect limit, the SSRF guard on every hop) still applies.
//
// Dialing goes through safeurl, so impersonation never buys a way past the
// SSRF guard: the address check runs on the connection actually opened.
func Transport() (http.RoundTripper, error) {
	return newTransport(false)
}

// newTransport is the test seam: a local httptest TLS server presents a
// self-signed certificate, so tests need to skip verification. Production
// callers go through Transport.
func newTransport(insecureSkipVerify bool) (http.RoundTripper, error) {
	opts := []tlsclient.HttpClientOption{
		tlsclient.WithClientProfile(profiles.DefaultClientProfile),
		// net/http follows redirects; letting tls-client do it too would hide
		// intermediate hops from Colly and from the SSRF guard.
		tlsclient.WithNotFollowRedirects(),
		tlsclient.WithCatchPanics(),
		// The deadline belongs to the caller's context (Colly's request
		// timeout); a second, independent timer here only causes surprises.
		tlsclient.WithTimeoutMilliseconds(0),
		tlsclient.WithDialContext(safeurl.DialContext),
	}
	if insecureSkipVerify {
		opts = append(opts, tlsclient.WithInsecureSkipVerify())
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), opts...)
	if err != nil {
		return nil, fmt.Errorf("create fingerprint client: %w", err)
	}
	return &roundTripper{
		client: client,
		// Plain HTTP has no TLS handshake to impersonate, and some Chrome
		// profiles refuse http:// outright. Route it through the guarded Go
		// transport so internal http:// sites keep working.
		plain: safeurl.Transport(),
	}, nil
}

// roundTripper adapts tls-client's fhttp-based client to net/http, which is
// what Colly and the rest of the codebase speak. The header and body types are
// shared shapes (map[string][]string, io.ReadCloser), so the conversion is a
// copy rather than a re-encode.
type roundTripper struct {
	client tlsclient.HttpClient
	plain  http.RoundTripper
}

func (r *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return r.plain.RoundTrip(withDefaultHeaders(req))
	}

	var body io.Reader
	if req.Body != nil {
		body = req.Body
	}

	freq, err := fhttp.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("fingerprint request: %w", err)
	}
	for key, values := range req.Header {
		for _, value := range values {
			freq.Header.Add(key, value)
		}
	}
	// Fill in the browser identity only where the caller left a header unset,
	// so an explicit request header always wins but a bare request can never
	// leave the UA and the handshake disagreeing.
	for key, value := range DefaultHeaders() {
		if freq.Header.Get(key) == "" {
			freq.Header.Set(key, value)
		}
	}
	if req.Host != "" {
		freq.Host = req.Host
	}

	resp, err := r.client.Do(freq)
	if err != nil {
		return nil, err
	}

	header := make(http.Header, len(resp.Header))
	for key, values := range resp.Header {
		header[key] = append([]string(nil), values...)
	}

	return &http.Response{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Proto:         resp.Proto,
		ProtoMajor:    resp.ProtoMajor,
		ProtoMinor:    resp.ProtoMinor,
		Header:        header,
		Body:          resp.Body,
		ContentLength: resp.ContentLength,
		Request:       req,
		// fhttp's transport decompresses gzip/br itself and reports it here;
		// forwarding the flag keeps net/http from trying again.
		Uncompressed: resp.Uncompressed,
	}, nil
}

// withDefaultHeaders returns a request whose unset navigation headers carry
// the browser identity. The header map is cloned so the caller's request is
// never mutated, as http.RoundTripper requires.
func withDefaultHeaders(req *http.Request) *http.Request {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	for key, value := range DefaultHeaders() {
		if clone.Header.Get(key) == "" {
			clone.Header.Set(key, value)
		}
	}
	return clone
}
