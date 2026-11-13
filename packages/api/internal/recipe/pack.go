package recipe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Michael-Obele/tomoshibi/internal/extract"
	"github.com/PuerkitoBio/goquery"
	"go.yaml.in/yaml/v3"
)

// ManifestName is the pack manifest file every pack must carry.
const ManifestName = "manifest.yaml"

// ManifestFile is one hashed file in a pack.
type ManifestFile struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
}

// Manifest is a pack's file list. The installer verifies it; a tampered pack
// is refused, because a recipe decides what the bridge may drive.
type Manifest struct {
	Name    string         `yaml:"name"`
	Version int            `yaml:"version"`
	Files   []ManifestFile `yaml:"files"`
}

// splitPackDir splits "pack@abc123" into its name and short sha.
func splitPackDir(base string) (name, sha string) {
	if i := strings.LastIndex(base, "@"); i > 0 {
		return base[:i], base[i+1:]
	}
	return base, ""
}

// RecipesDir is the install root inside the state dir.
func RecipesDir(stateDir string) string { return filepath.Join(stateDir, "recipes") }

// LoadManifest reads and lightly validates the manifest.
func LoadManifest(dir string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return nil, fmt.Errorf("pack: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("pack: manifest: %w", err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("pack: manifest.name is required")
	}
	if len(m.Files) == 0 {
		return nil, fmt.Errorf("pack %s: manifest.files is empty; a pack proves its own contents", m.Name)
	}
	return &m, nil
}

// VerifyManifest hash-checks every file against the manifest. Both directions
// matter: an unlisted file is as much a failure as a mismatched one.
func VerifyManifest(dir string) (*Manifest, error) {
	m, err := LoadManifest(dir)
	if err != nil {
		return nil, err
	}
	listed := map[string]string{}
	for _, f := range m.Files {
		listed[filepath.ToSlash(f.Path)] = f.SHA256
	}

	seen := map[string]bool{}
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skipEntry(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if rel == ManifestName {
			return nil
		}
		want, ok := listed[rel]
		if !ok {
			return fmt.Errorf("pack %s: file %s is not listed in the manifest", m.Name, rel)
		}
		got, err := fileSHA256(path)
		if err != nil {
			return err
		}
		if !strings.EqualFold(got, want) {
			return fmt.Errorf("pack %s: %s hash mismatch (manifest %s, file %s)", m.Name, rel, want[:12], got[:12])
		}
		seen[rel] = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	for rel := range listed {
		if !seen[rel] {
			return nil, fmt.Errorf("pack %s: manifest lists %s but the file is missing", m.Name, rel)
		}
	}
	return m, nil
}

// skipEntry filters things that are never pack content.
func skipEntry(name string) bool {
	return name == ".git" || name == "node_modules" || (strings.HasPrefix(name, ".") && name != ".")
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// treeSHA256 hashes a directory's content (paths + file hashes), giving local
// packs a stable identifier without git.
func treeSHA256(dir string) (string, error) {
	h := sha256.New()
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && skipEntry(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if skipEntry(d.Name()) {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	for _, p := range paths {
		rel, _ := filepath.Rel(dir, p)
		sum, err := fileSHA256(p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%s\n", filepath.ToSlash(rel), sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Install verifies a pack and copies it into the state dir under
// <name>@<sha12>. src is either a local directory or a git URL with an
// optional "#ref" suffix. A pack that is already installed is a no-op.
// ctx bounds the git calls: an unreachable host must not hang the CLI forever.
func Install(ctx context.Context, src, stateDir string) (*Manifest, string, error) {
	base, ref, _ := strings.Cut(src, "#")
	if base == "" {
		return nil, "", fmt.Errorf("pack: empty source")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	dir := base
	var sha string
	if looksLikeGit(base) {
		tmp, err := os.MkdirTemp("", "tomoshibi-pack-*")
		if err != nil {
			return nil, "", err
		}
		defer os.RemoveAll(tmp)

		clone := []string{"clone", "--depth", "1"}
		if ref != "" {
			clone = append(clone, "--branch", ref)
		}
		clone = append(clone, base, tmp)
		if out, err := runGit(ctx, clone...); err != nil {
			// --branch only accepts a branch/tag; a raw commit needs fetch+checkout.
			if ref == "" {
				return nil, "", fmt.Errorf("pack: git clone: %v: %s", err, strings.TrimSpace(string(out)))
			}
			if out, err := runGit(ctx, "clone", base, tmp); err != nil {
				return nil, "", fmt.Errorf("pack: git clone: %v: %s", err, strings.TrimSpace(string(out)))
			}
			if out, err := runGit(ctx, "-C", tmp, "checkout", ref); err != nil {
				return nil, "", fmt.Errorf("pack: git checkout %s: %v: %s", ref, err, strings.TrimSpace(string(out)))
			}
		}
		out, err := runGit(ctx, "-C", tmp, "rev-parse", "HEAD")
		if err != nil {
			return nil, "", fmt.Errorf("pack: git rev-parse: %w", err)
		}
		sha = strings.TrimSpace(string(out))
		dir = tmp
	} else {
		if st, err := os.Stat(base); err != nil || !st.IsDir() {
			return nil, "", fmt.Errorf("pack: %s is not a directory or git URL", base)
		}
		sum, err := treeSHA256(base)
		if err != nil {
			return nil, "", err
		}
		sha = sum
	}

	m, err := VerifyManifest(dir)
	if err != nil {
		return nil, "", err
	}
	// A pack's own name picks the destination directory, so it has to be a
	// bare path component: "../../../.config/x" must not escape the state dir.
	if !filepath.IsLocal(m.Name) || strings.ContainsAny(m.Name, "/\\") {
		return nil, "", fmt.Errorf("pack: manifest name %q must be a single path component", m.Name)
	}
	if len(sha) > 12 {
		sha = sha[:12]
	}
	dest := filepath.Join(RecipesDir(stateDir), m.Name+"@"+sha)
	if _, err := os.Stat(dest); err == nil {
		return m, dest, nil // already installed
	}
	if err := copyTree(dir, dest); err != nil {
		return nil, "", err
	}
	return m, dest, nil
}

// runGit runs one git command under the caller's context.
func runGit(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "git", args...).CombinedOutput()
}

// Remove uninstalls one pack directory (name@sha).
func Remove(stateDir, pack string) error {
	if strings.ContainsAny(pack, "/\\") || strings.Contains(pack, "..") {
		return fmt.Errorf("pack: invalid pack name %q", pack)
	}
	dest := filepath.Join(RecipesDir(stateDir), pack)
	if _, err := os.Stat(dest); err != nil {
		return fmt.Errorf("pack: %s is not installed", pack)
	}
	return os.RemoveAll(dest)
}

// PackInfo summarises one installed pack for `recipes list`.
type PackInfo struct {
	Dir     string
	Name    string
	SHA     string
	Recipes []string
}

// List returns every installed pack with its recipe ids.
func List(stateDir string) ([]PackInfo, error) {
	entries, err := os.ReadDir(RecipesDir(stateDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []PackInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name, sha := splitPackDir(e.Name())
		info := PackInfo{Dir: e.Name(), Name: name, SHA: sha}
		sites, _ := os.ReadDir(filepath.Join(RecipesDir(stateDir), e.Name(), "sites"))
		for _, s := range sites {
			if !s.IsDir() && strings.HasSuffix(s.Name(), ".yaml") {
				info.Recipes = append(info.Recipes, strings.TrimSuffix(s.Name(), ".yaml"))
			}
		}
		sort.Strings(info.Recipes)
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out, nil
}

// FixtureResult is one fixture's verdict from the offline replay.
type FixtureResult struct {
	RecipeID string
	URL      string
	File     string
	Pass     bool
	Detail   string
}

// VerifyFixtures replays every saved page through the recipe's extractor and
// checks the expectations. It never touches the network: a fixture is the
// recipe's proof that it still parses, and the run is offline by design.
func VerifyFixtures(r *Recipe) ([]FixtureResult, error) {
	if len(r.Fixtures) == 0 {
		return nil, fmt.Errorf("recipe %s: no fixtures; a recipe without a saved page cannot fail loudly", r.ID)
	}
	var results []FixtureResult
	for _, f := range r.Fixtures {
		res := FixtureResult{RecipeID: r.ID, URL: f.URL, File: f.File, Pass: true}
		// A fixture path comes from the pack, so it must stay inside the pack:
		// "../../.config/tomoshi/bridge.json" is not a fixture, it is a file
		// read dressed up as one.
		path := filepath.Join(r.PackDir, filepath.FromSlash(f.File))
		if !filepath.IsLocal(filepath.FromSlash(f.File)) {
			res.Pass, res.Detail = false, "fixture path escapes the pack directory"
			results = append(results, res)
			continue
		}
		if rel, relErr := filepath.Rel(r.PackDir, path); relErr != nil || strings.HasPrefix(rel, "..") {
			res.Pass, res.Detail = false, "fixture path escapes the pack directory"
			results = append(results, res)
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			res.Pass, res.Detail = false, fmt.Sprintf("fixture missing: %v", err)
			results = append(results, res)
			continue
		}
		html := string(raw)

		// The page text is used for the contains/absent checks: block pages
		// arrive as HTTP 200 and are only recognisable from their content.
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err != nil {
			res.Pass, res.Detail = false, fmt.Sprintf("fixture unparseable: %v", err)
			results = append(results, res)
			continue
		}
		text := doc.Text()

		var extracted map[string]any
		if len(r.Extract.Schema) > 0 {
			extracted, _ = extract.Apply(html, r.Extract.Schema)
		}
		for key, want := range f.Expect {
			got := strings.TrimSpace(fmt.Sprint(extracted[key]))
			if got != strings.TrimSpace(want) {
				res.Pass = false
				res.Detail = fmt.Sprintf("field %q: want %q, got %q", key, want, got)
				break
			}
		}
		if res.Pass {
			for _, marker := range r.Expect.Absent {
				if strings.Contains(text, marker) {
					res.Pass = false
					res.Detail = fmt.Sprintf("fixture contains a block marker %q; the saved page is a challenge, not content", marker)
					break
				}
			}
		}
		if res.Pass {
			for _, needle := range r.Expect.Contains {
				if !strings.Contains(text, needle) {
					res.Pass = false
					res.Detail = fmt.Sprintf("fixture is missing expected text %q", needle)
					break
				}
			}
		}
		results = append(results, res)
	}
	return results, nil
}

func looksLikeGit(src string) bool {
	return strings.Contains(src, "://") || strings.HasPrefix(src, "git@") || strings.HasSuffix(src, ".git")
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if skipEntry(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	})
}
