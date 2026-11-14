// Package recipe implements the recipe layer: per-site YAML that names a
// lane (plain / official / import / borrow), the actions to run, the
// extraction rules, the expectations that prove a page is real, and the
// fixtures that prove the recipe still parses without touching the network.
//
// Recipes are data, never code. They are installed from git packs into the
// state dir and matched to a request by host.
package recipe

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"go.yaml.in/yaml/v3"
)

// Lane names what leaves the browser and what the site is asked to tolerate.
const (
	LanePlain    = "plain"
	LaneOfficial = "official"
	LaneImport   = "import"
	LaneBorrow   = "borrow"
)

// IO selects the bridge's input layer: scripting (default, clean, works on
// Firefox) or turbo (chrome.debugger: trusted input, full-page screenshots).
const (
	IOScripting = "scripting"
	IOTurbo     = "turbo"
)

// Action is one page interaction the bridge performs, in order.
type Action struct {
	Type     string `yaml:"type" json:"type"`
	Selector string `yaml:"selector,omitempty" json:"selector,omitempty"`
	Text     string `yaml:"text,omitempty" json:"text,omitempty"`
	Key      string `yaml:"key,omitempty" json:"key,omitempty"`
	URL      string `yaml:"url,omitempty" json:"url,omitempty"`
	Script   string `yaml:"script,omitempty" json:"script,omitempty"`
	Ms       int    `yaml:"ms,omitempty" json:"ms,omitempty"`
	Px       int    `yaml:"px,omitempty" json:"px,omitempty"`
}

// Pace is the recipe's cadence: a floor between two runs against the same
// recipe. The borrow lane drives a real person's browser, so the pack that
// hammers a site is the pack that gets the user's session flagged.
type Pace struct {
	MinIntervalMS int `yaml:"min_interval_ms,omitempty"`
}

// Expect is what proves a fetched page is real content and not a challenge.
// Absent is the load-bearing half: block pages arrive as HTTP 200.
type Expect struct {
	// Status is the status the page should have been served with. It is only
	// checked when the browser reports one: Chrome reports the status of a
	// same-origin navigation and may report 0 for a cross-origin one, so an
	// unreported status is not a failure. contains/absent always apply.
	Status   int      `yaml:"status,omitempty"`
	Contains []string `yaml:"contains,omitempty"`
	Absent   []string `yaml:"absent,omitempty"`
}

// Fixture is a saved page plus the extraction values it must still produce.
type Fixture struct {
	URL    string            `yaml:"url"`
	File   string            `yaml:"file"`
	Expect map[string]string `yaml:"expect,omitempty"`
}

type matchSpec struct {
	Domains []string `yaml:"domains"`
}

type accessSpec struct {
	Lane string `yaml:"lane"`
	IO   string `yaml:"io,omitempty"`
	Pace *Pace  `yaml:"pace,omitempty"`
	// AllowPrivate lets this recipe reach hosts that the SSRF guard blocks
	// (a homelab dashboard at 192.168.x.x is a legitimate borrow-lane target).
	// It only applies to hosts the recipe itself declares in match.domains, so
	// it cannot be borrowed to reach anything else.
	AllowPrivate bool `yaml:"allow_private,omitempty"`
}

type requestSpec struct {
	Actions      []Action `yaml:"actions,omitempty"`
	WaitSelector string   `yaml:"wait_selector,omitempty"`
	SettleMS     int      `yaml:"settle_ms,omitempty"`
	TimeoutMS    int      `yaml:"timeout_ms,omitempty"`
}

type extractSpec struct {
	Schema map[string]domain.ExtractField `yaml:"schema,omitempty"`
}

// Recipe is one site's access + extraction definition.
type Recipe struct {
	ID       string      `yaml:"id"`
	Version  int         `yaml:"version"`
	Match    matchSpec   `yaml:"match"`
	Access   accessSpec  `yaml:"access"`
	Request  requestSpec `yaml:"request,omitempty"`
	Extract  extractSpec `yaml:"extract,omitempty"`
	Expect   Expect      `yaml:"expect,omitempty"`
	Fixtures []Fixture   `yaml:"fixtures"`

	// Set at load time; never parsed from the file.
	PackName string `yaml:"-"`
	PackSHA  string `yaml:"-"`
	PackDir  string `yaml:"-"`
}

// Ref is the id@version pair used in provenance and the audit log.
func (r *Recipe) Ref() string { return fmt.Sprintf("%s@%d", r.ID, r.Version) }

// MatchesHost reports whether this recipe claims a hostname. Callers that must
// reason about the target before fetching (the SSRF guard's opt-out) use this
// rather than re-implementing pattern matching.
func (r *Recipe) MatchesHost(host string) bool {
	for _, d := range r.Match.Domains {
		if hostMatches(d, host) {
			return true
		}
	}
	return false
}

// validActionTypes is the bridge's closed vocabulary. It has to match what the
// extension's two layers actually implement; a recipe is rejected at load time
// rather than failing one action silently at run time.
var validActionTypes = map[string]bool{
	"wait_selector": true, "wait_ms": true, "scroll_to_bottom": true,
	"scroll_by": true, "scroll_down": true, "click": true, "type": true,
	"press": true, "navigate": true, "evaluate": true, "wait_for_function": true,
}

// Parse reads one recipe from YAML and validates it.
//
// Unknown keys are an error, not a shrug: a recipe that asks for a knob the
// engine does not have (a page budget, a profile, a field an older pack used)
// must fail at load time, because a recipe whose rules are silently dropped is
// worse than one that refuses to load.
func Parse(raw []byte) (*Recipe, error) {
	var r Recipe
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("recipe: parse: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

// Validate rejects a recipe that could not run, at load time, loudly.
func (r *Recipe) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("recipe: id is required")
	}
	if r.Version < 1 {
		return fmt.Errorf("recipe %s: version must be >= 1", r.ID)
	}
	if len(r.Match.Domains) == 0 {
		return fmt.Errorf("recipe %s: match.domains is required", r.ID)
	}
	switch r.Access.Lane {
	case LanePlain, LaneOfficial, LaneImport, LaneBorrow:
	default:
		return fmt.Errorf("recipe %s: unknown lane %q", r.ID, r.Access.Lane)
	}
	switch r.Access.IO {
	case "", IOScripting, IOTurbo:
	default:
		return fmt.Errorf("recipe %s: unknown io %q (want scripting or turbo)", r.ID, r.Access.IO)
	}
	for i, a := range r.Request.Actions {
		if !validActionTypes[a.Type] {
			return fmt.Errorf("recipe %s: action %d has unknown type %q", r.ID, i, a.Type)
		}
		// The scripting layer is DOM events; a JS expression needs the debugger
		// layer, so a recipe that asks for it under scripting can never run.
		if r.IO() != IOTurbo && (a.Type == "evaluate" || a.Type == "wait_for_function") {
			return fmt.Errorf("recipe %s: action %d is %q, which needs io: turbo", r.ID, i, a.Type)
		}
	}
	if len(r.Fixtures) == 0 {
		return fmt.Errorf("recipe %s: at least one fixture is required; a recipe without a saved page cannot fail loudly", r.ID)
	}
	for i, f := range r.Fixtures {
		if f.URL == "" || f.File == "" {
			return fmt.Errorf("recipe %s: fixture %d needs url and file", r.ID, i)
		}
	}
	return nil
}

// IO returns the effective IO layer (default scripting).
func (r *Recipe) IO() string {
	if r.Access.IO == "" {
		return IOScripting
	}
	return r.Access.IO
}

// hostMatches reports whether host is covered by a pattern: exact match or a
// "*.suffix" match.
func hostMatches(pattern, host string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	host = strings.ToLower(host)
	if pattern == host {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		return strings.HasSuffix(host, pattern[1:])
	}
	return false
}

// Registry holds every installed recipe and matches them to hosts. It reloads
// lazily when the recipes dir changes, so `recipes add` works without a
// restart.
type Registry struct {
	mu       sync.RWMutex
	dir      string
	mtime    time.Time
	byID     map[string]*Recipe
	all      []*Recipe
	warnings []string
}

// LoadRegistry reads every installed pack. A missing dir is an empty registry:
// the bridge simply has nothing to route yet — but a dir that exists and
// cannot be listed is a warning, not silence.
func LoadRegistry(stateDir string) *Registry {
	reg := &Registry{dir: filepath.Join(stateDir, "recipes"), byID: map[string]*Recipe{}}
	reg.reload()
	return reg
}

func (reg *Registry) reload() {
	byID := map[string]*Recipe{}
	var all []*Recipe
	var warnings []string
	var mtime time.Time

	// Stat before listing, and record the mtime we saw BEFORE reading: statting
	// after the read can record a newer timestamp than the content actually
	// read, which would make the next reload skip a pack installed in the gap.
	info, statErr := os.Stat(reg.dir)
	if statErr == nil {
		mtime = info.ModTime()
	}
	entries, err := os.ReadDir(reg.dir)
	if err != nil {
		if !os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("recipes dir %s: %v", reg.dir, err))
		}
	} else {
		packDirs := []string{}
		for _, e := range entries {
			if e.IsDir() {
				packDirs = append(packDirs, filepath.Join(reg.dir, e.Name()))
			}
		}
		sort.Strings(packDirs)
		for _, packDir := range packDirs {
			packName, packSHA := splitPackDir(filepath.Base(packDir))
			sitesDir := filepath.Join(packDir, "sites")
			files, err := os.ReadDir(sitesDir)
			if err != nil {
				continue
			}
			for _, f := range files {
				if f.IsDir() || !strings.HasSuffix(f.Name(), ".yaml") {
					continue
				}
				path := filepath.Join(sitesDir, f.Name())
				raw, err := os.ReadFile(path)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
					continue
				}
				r, err := Parse(raw)
				if err != nil {
					warnings = append(warnings, err.Error())
					continue
				}
				r.PackName, r.PackSHA, r.PackDir = packName, packSHA, packDir
				if prev, dup := byID[r.ID]; dup {
					warnings = append(warnings, fmt.Sprintf("recipe %s installed twice (%s, %s); keeping the first", r.ID, prev.PackName, packName))
					continue
				}
				byID[r.ID] = r
				all = append(all, r)
			}
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	reg.mu.Lock()
	reg.byID, reg.all, reg.warnings = byID, all, warnings
	reg.mtime = mtime
	reg.mu.Unlock()
}

// reloadIfStale picks up an install/remove without a restart.
func (reg *Registry) reloadIfStale() {
	info, err := os.Stat(reg.dir)
	reg.mu.RLock()
	current := reg.mtime
	loaded := reg.all != nil
	reg.mu.RUnlock()
	if err != nil {
		// The dir is gone: an empty registry, not the last one we happened to
		// load. Otherwise `rm -rf recipes` would keep serving deleted packs.
		if os.IsNotExist(err) && loaded {
			reg.reload()
		}
		return
	}
	if info.ModTime() != current || !loaded {
		reg.reload()
	}
}

// Match returns the most specific recipe covering host, or nil.
//
// Specificity, in order: an exact host beats any wildcard (a recipe for
// `m.example.com` must win over `*.example.com`), then more labels, then a
// longer pattern. The recipe id breaks the remaining ties, so the winner never
// depends on directory or map order.
func (reg *Registry) Match(host string) *Recipe {
	reg.reloadIfStale()
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	var best *Recipe
	bestScore := -1
	for _, r := range reg.all {
		for _, p := range r.Match.Domains {
			if !hostMatches(p, host) {
				continue
			}
			score := specificity(p)
			switch {
			case score > bestScore:
			case score == bestScore && best != nil && r.ID < best.ID:
			default:
				continue
			}
			best, bestScore = r, score
		}
	}
	return best
}

// specificity ranks one host pattern: an exact host beats any suffix pattern,
// then more labels, then a longer pattern.
func specificity(pattern string) int {
	suffix := strings.TrimPrefix(pattern, "*.")
	exact := pattern == suffix // no "*." prefix: an exact host
	score := strings.Count(suffix, ".") * 1000
	if exact {
		score += 1_000_000
	}
	return score + len(pattern)
}

// ByID finds a recipe by its id.
func (reg *Registry) ByID(id string) (*Recipe, bool) {
	reg.reloadIfStale()
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	r, ok := reg.byID[id]
	return r, ok
}

// All returns every installed recipe, sorted by id.
func (reg *Registry) All() []*Recipe {
	reg.reloadIfStale()
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	out := make([]*Recipe, len(reg.all))
	copy(out, reg.all)
	return out
}

// Warnings reports recipes that failed to load (invalid YAML, bad lane, ...).
func (reg *Registry) Warnings() []string {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	out := make([]string, len(reg.warnings))
	copy(out, reg.warnings)
	return out
}
