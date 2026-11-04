package scraper

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Challenge identifies an anti-bot interstitial found in a response body.
// Vendor is a short lowercase name ("cloudflare", "datadome", …) and Marker is
// the literal that matched, kept for logs and for the solver addon.
type Challenge struct {
	Vendor string
	Marker string
}

// challengeMarker is one detection literal. Strong markers are unique enough
// to identify a challenge on their own; weak ones also require corroboration
// (an error status or a challenge-shaped title) so an article *about* bot
// walls is not mistaken for one.
type challengeMarker struct {
	vendor string
	text   string
	strong bool
}

// challengeMaxBody bounds detection. Challenge interstitials are small; a
// large document that merely quotes one of these strings (a security blog, a
// test fixture) is content, not a wall.
const challengeMaxBody = 256 << 10

// weakMarkerMaxBody is the size below which a weak marker plus a challenge
// title is credible.
const weakMarkerMaxBody = 32 << 10

var challengeMarkers = []challengeMarker{
	// Cloudflare
	{"cloudflare", "<title>just a moment...</title>", true},
	{"cloudflare", "_cf_chl_opt", true},
	{"cloudflare", "cf-browser-verification", true},
	{"cloudflare", "challenge-platform", true},
	{"cloudflare", "attention required! | cloudflare", true},
	{"cloudflare", "checking your browser before accessing", true},
	{"cloudflare", "enable javascript and cookies to continue", true},
	{"cloudflare", "cf-chl-", true},
	// DataDome
	{"datadome", "captcha-delivery.com", true},
	{"datadome", "datadome", false},
	// PerimeterX / HUMAN
	{"perimeterx", "px-captcha", true},
	{"perimeterx", "_pxappid", true},
	{"perimeterx", "press & hold", false},
	// Imperva / Incapsula
	{"imperva", "incapsula incident id", true},
	{"imperva", "_incapsula_resource", true},
	{"imperva", "request unsuccessful. incapsula", true},
	// Akamai
	{"akamai", "reference #18.", true},
	{"akamai", "access denied", false},
	// AWS WAF
	{"aws-waf", "awswaf", true},
	{"aws-waf", "aws-waf-token", true},
	// Sucuri
	{"sucuri", "sucuri website firewall", true},
	// DDoS-Guard
	{"ddos-guard", "ddos-guard", true},
	// Generic
	{"generic", "verify you are human", false},
	{"generic", "are you a robot", false},
	{"generic", "unusual traffic from your computer network", true},
	{"generic", "please complete the security check", false},
	{"generic", "checking if the site connection is secure", true},
}

// detectChallenge reports whether body is an anti-bot challenge page rather
// than content. A nil result means the body is safe to treat as a page.
//
// This is the single most important guard in the pipeline: challenge pages
// return HTTP 200, so without it the API would hand an agent a confident
// summary of "Just a moment..." and call it a successful scrape.
func detectChallenge(body []byte, status int) *Challenge {
	if len(body) == 0 || len(body) > challengeMaxBody {
		return nil
	}
	lower := bytes.ToLower(body)
	thin := len(body) < weakMarkerMaxBody
	titled := hasChallengeTitle(lower)

	for _, m := range challengeMarkers {
		if !bytes.Contains(lower, []byte(m.text)) {
			continue
		}
		switch {
		case m.strong && len(body) <= weakMarkerMaxBody:
			return &Challenge{Vendor: m.vendor, Marker: m.text}
		case m.strong && (status >= http.StatusBadRequest || looksLikeChallengeDoc(lower)):
			return &Challenge{Vendor: m.vendor, Marker: m.text}
		case !m.strong && (status >= http.StatusBadRequest || (thin && titled)):
			return &Challenge{Vendor: m.vendor, Marker: m.text}
		}
	}
	return nil
}

// looksLikeChallengeDoc is the corroboration for a strong marker found in a
// large document. Interstitials are bare pages; an article *about* bot walls
// quotes the same strings but is built out of paragraphs. Requiring structure
// keeps a security blog post from being rejected as a block page.
func looksLikeChallengeDoc(lower []byte) bool {
	return bytes.Count(lower, []byte("<p")) <= 2 &&
		!bytes.Contains(lower, []byte("<article"))
}

// hasChallengeTitle reports whether the document title reads like a block
// page. It is the corroborating signal for weak markers.
func hasChallengeTitle(lower []byte) bool {
	start := bytes.Index(lower, []byte("<title"))
	if start < 0 {
		return false
	}
	end := bytes.Index(lower[start:], []byte("</title>"))
	if end < 0 {
		return false
	}
	title := lower[start : start+end]
	for _, word := range [][]byte{
		[]byte("moment"), []byte("denied"), []byte("captcha"),
		[]byte("verify"), []byte("robot"), []byte("attention"),
		[]byte("blocked"), []byte("security"), []byte("challenge"),
	} {
		if bytes.Contains(title, word) {
			return true
		}
	}
	return false
}

// parseRetryAfter converts a Retry-After header into a wait. It accepts both
// forms the RFC allows (delta-seconds and HTTP-date) and caps the result so a
// hostile origin cannot stall a crawl indefinitely.
func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	const maxWait = 10 * time.Minute

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		wait := time.Duration(seconds) * time.Second
		if wait > maxWait {
			return maxWait
		}
		return wait
	}
	if when, err := http.ParseTime(value); err == nil {
		wait := time.Until(when)
		if wait <= 0 {
			return 0
		}
		if wait > maxWait {
			return maxWait
		}
		return wait
	}
	return 0
}
