package engines

import (
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/bogdanfinn/tls-client/profiles"
	"go.yaml.in/yaml/v3"
)

// defaultRegistry is the bundled roster (opt-in default per the user's
// config-over-hardcoded rule; SEARCH_ENGINES_PATH overrides it wholesale).
//
//go:embed registry.default.yaml
var defaultRegistry []byte

// Spec is one engine entry in the registry YAML.
type Spec struct {
	// Name is the engine identifier (SearXNG-style engine string in output).
	Name string `yaml:"name"`
	// Categories is the subset of general|news|it this engine serves.
	Categories []string `yaml:"categories"`
	// Weight orders the engine in the merged result list (higher first).
	Weight int `yaml:"weight"`
	// Enabled defaults to true; false keeps the entry documented but out
	// of the live roster (e.g. Google HTML ships disabled, plan R2).
	Enabled *bool `yaml:"enabled"`
	// TimeoutMS caps a single upstream request (default 4000).
	TimeoutMS int `yaml:"timeout_ms"`
	// MinIntervalMS spaces successive requests to THIS engine across
	// concurrent queries (0 = none). DuckDuckGo's anomaly page triggers
	// when we fire too fast — observed live 2026-09-26.
	MinIntervalMS int `yaml:"min_interval_ms"`
	// Profile selects the TLS fingerprint: "" = plain Go net/http,
	// "chrome" = default browser profile, or an explicit profile name
	// from tls-client's MappedTLSClients ("chrome_131", ...).
	Profile string `yaml:"profile"`
	// Proxy names a ProxyPool group ("webshare", "tier1", "tier2").
	// Unconfigured groups degrade to a direct request.
	Proxy string `yaml:"proxy"`
	// RequiresEnv skips the engine cleanly when this env var is empty.
	RequiresEnv string `yaml:"requires_env"`
	// SiteOperator declares that this engine understands the `site:` query
	// operator, so domain filters can be pushed into the query text instead
	// of only being applied after the fact. Defaults to false.
	//
	// Only set it on engines verified to implement the operator. Leaving it
	// off costs recall but stays correct; setting it on an engine that does
	// not (a plain JSON/RSS API such as github or npm) injects a token the
	// engine reads as a literal search term and returns nonsense. The
	// post-filter in the parent package runs either way, so this flag can
	// never make results incorrect — only the query text sent upstream.
	SiteOperator bool `yaml:"site_operator"`
	// BlockMarkers are lowercased substrings that, in a 200 body, mark the
	// response as a captcha/anomaly page (engine returns ErrBlocked).
	BlockMarkers []string `yaml:"block_markers"`
	// Request builds the upstream HTTP call.
	Request RequestSpec `yaml:"request"`
	// Parse describes how the response body becomes []Result.
	Parse ParseSpec `yaml:"parse"`
}

// RequestSpec is the declarative upstream request.
type RequestSpec struct {
	// Method defaults to GET.
	Method string `yaml:"method"`
	// URL supports {{query}}, {{pageno}}, {{language}}, {{time_range}}
	// placeholders (query is URL-encoded). Body uses the same placeholders.
	URL     string            `yaml:"url"`
	Body    string            `yaml:"body"`
	Headers map[string]string `yaml:"headers"`
}

// ParseSpec selects a parser and maps fields. Exactly one of Type: css|rss|json.
type ParseSpec struct {
	// Type is "css" (goquery), "rss" (RSS 2.0/Atom items), or "json".
	Type string `yaml:"type"`
	// Item is the per-result selector (css) or dot-path to the array (json).
	// Empty for rss (the item list is implicit) and for a JSON root array.
	Item string `yaml:"item"`
	// Fields maps result keys: title/url (required), content/published
	// (optional).
	Fields map[string]FieldSpec `yaml:"fields"`
}

// FieldSpec extracts one field.
//
// css mode: Sel is the selector under the item node, Attr is "text", "html"
// or an attribute name, Exclude drops nested selectors before text.
// json/rss mode: Path is a dot-path to the value (rss paths are element
// names, case-insensitive).
// Shared: Template builds a URL from {{field}} placeholders (Encode URL-
// encodes substitutions), URLParam decodes redirector links
// (uddg | bing_u | url), Join concatenates an array of {key} objects
// (mwmbl's segmented titles), OrTemplate is used when Path is empty,
// StripHTML reduces HTML to plain text.
type FieldSpec struct {
	Sel        string `yaml:"sel"`
	Attr       string `yaml:"attr"`
	Exclude    string `yaml:"exclude"`
	Path       string `yaml:"path"`
	Template   string `yaml:"template"`
	Encode     bool   `yaml:"encode"`
	URLParam   string `yaml:"urlparam"`
	Join       string `yaml:"join"`
	OrTemplate string `yaml:"or_template"`
	StripHTML  bool   `yaml:"strip_html"`
}

// Registry is the validated engine roster, ordered by weight (then name)
// within each category.
type Registry struct {
	specs  []*Spec
	byName map[string]*Spec
	fusion Fusion
}

// Fusion holds ranking policy that belongs to the roster as a whole rather
// than to any single engine.
type Fusion struct {
	// MaxPerDomain caps how many results ONE engine may contribute from a
	// single registrable domain ("example.com", not "www.example.com").
	//
	// Without it a metasearch has no defence against one source monopolising
	// a page: measured on live engine output, a single encyclopaedia engine
	// held 8 of the top 10 slots on a shopping query, purely because
	// weightFor hands the top-weighted engine ~1.0 and every other engine's
	// best hit scores below the leader's eighth. Capping matches what the
	// major engines do — Google and Bing show at most two results from one
	// domain on a SERP (Akritidis et al., JSS 2010).
	//
	// Keyed by category with an optional "default" fallback. Zero means
	// uncapped, which is the right setting for "it": GitHub legitimately
	// answers a code query with twenty repos, all on github.com, and capping
	// those would discard the answer rather than diversify it.
	MaxPerDomain map[string]int `yaml:"max_per_domain"`
}

// Fusion returns the roster-level ranking policy.
func (r *Registry) Fusion() Fusion {
	if r == nil {
		return Fusion{}
	}
	return r.fusion
}

// MaxPerDomainFor resolves the cap for a category, falling back to
// "default" and then to uncapped.
func (r *Registry) MaxPerDomainFor(category string) int {
	if r == nil || len(r.fusion.MaxPerDomain) == 0 {
		return 0
	}
	if n, ok := r.fusion.MaxPerDomain[category]; ok {
		return n
	}
	if n, ok := r.fusion.MaxPerDomain["default"]; ok {
		return n
	}
	return 0
}

// doc is the on-disk YAML shape.
type doc struct {
	Version int     `yaml:"version"`
	Fusion  Fusion  `yaml:"fusion"`
	Engines []*Spec `yaml:"engines"`
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

var validCategories = map[string]bool{"general": true, "news": true, "it": true}
var validParseTypes = map[string]bool{"css": true, "rss": true, "json": true}

// LoadRegistry reads a registry file, or the bundled default when path is
// "" (SEARCH_ENGINES_PATH). It validates strictly: unknown fields, duplicate
// names, bad categories, and unknown TLS profiles fail loudly rather than
// shipping a silently broken roster.
func LoadRegistry(path string) (*Registry, error) {
	raw := defaultRegistry
	src := "bundled registry.default.yaml"
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read engine registry %s: %w", path, err)
		}
		raw, src = b, path
	}
	var d doc
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("parse %s: %w", src, err)
	}
	if d.Version != 1 {
		return nil, fmt.Errorf("%s: unsupported registry version %d (want 1)", src, d.Version)
	}
	for cat, n := range d.Fusion.MaxPerDomain {
		if cat != "default" && !validCategories[cat] {
			return nil, fmt.Errorf("%s: fusion.max_per_domain: unknown category %q (want default|general|news|it)", src, cat)
		}
		if n < 0 {
			return nil, fmt.Errorf("%s: fusion.max_per_domain.%s: must be >= 0 (0 = uncapped)", src, cat)
		}
	}
	reg := &Registry{byName: make(map[string]*Spec, len(d.Engines)), fusion: d.Fusion}
	for i, s := range d.Engines {
		if err := validateSpec(s); err != nil {
			return nil, fmt.Errorf("%s: engine #%d: %w", src, i+1, err)
		}
		if _, dup := reg.byName[s.Name]; dup {
			return nil, fmt.Errorf("%s: duplicate engine name %q", src, s.Name)
		}
		reg.specs = append(reg.specs, s)
		reg.byName[s.Name] = s
	}
	if len(reg.specs) == 0 {
		return nil, fmt.Errorf("%s: registry has no engines", src)
	}
	return reg, nil
}

func validateSpec(s *Spec) error {
	if s == nil {
		return fmt.Errorf("nil spec")
	}
	if !nameRe.MatchString(s.Name) {
		return fmt.Errorf("invalid name %q (want lowercase [a-z0-9_-])", s.Name)
	}
	if len(s.Categories) == 0 {
		return fmt.Errorf("%s: at least one category required", s.Name)
	}
	for _, c := range s.Categories {
		if !validCategories[c] {
			return fmt.Errorf("%s: unknown category %q (want general|news|it)", s.Name, c)
		}
	}
	if s.Profile != "" && s.Profile != "chrome" {
		if _, ok := profiles.MappedTLSClients[s.Profile]; !ok {
			return fmt.Errorf("%s: unknown TLS profile %q", s.Name, s.Profile)
		}
	}
	if s.Request.URL == "" {
		return fmt.Errorf("%s: request.url required", s.Name)
	}
	if !validParseTypes[s.Parse.Type] {
		return fmt.Errorf("%s: unknown parse.type %q (want css|rss|json)", s.Name, s.Parse.Type)
	}
	if _, ok := s.Parse.Fields["title"]; !ok {
		return fmt.Errorf("%s: parse.fields.title required", s.Name)
	}
	if _, ok := s.Parse.Fields["url"]; !ok {
		return fmt.Errorf("%s: parse.fields.url required", s.Name)
	}
	if s.Parse.Type == "css" && s.Parse.Item == "" {
		return fmt.Errorf("%s: css parse requires item selector", s.Name)
	}
	return nil
}

// IsEnabled reports whether the spec is live (Enabled field absent or true).
func (s *Spec) IsEnabled() bool { return s.Enabled == nil || *s.Enabled }

// Spec returns the named spec (nil when absent).
func (r *Registry) Spec(name string) *Spec { return r.byName[name] }

// Specs returns all specs ordered by weight desc, then name — the merge
// order used by the fan-out layer.
func (r *Registry) Specs() []*Spec {
	out := append([]*Spec(nil), r.specs...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// EnabledSpecs returns the live roster in merge order.
func (r *Registry) EnabledSpecs() []*Spec {
	all := r.Specs()
	out := make([]*Spec, 0, len(all))
	for _, s := range all {
		if s.IsEnabled() {
			out = append(out, s)
		}
	}
	return out
}
