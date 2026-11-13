// Command tomoshibi is the bridge CLI: it manages pairing state, recipe packs,
// and reads the access log. It never needs the server running: every command
// works directly on the state dir (and the running server notices the edit), so
// a recipe can be verified offline before the API ever sees it.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/bridge"
	"github.com/Michael-Obele/tomoshibi/internal/recipe"
)

const usage = `tomoshibi - bridge pairing and recipe manager

usage: tomoshibi <command>

  bridge code                 print the current pairing code and port
  bridge status               print paired devices and last-seen times
  bridge unpair [name|all]    revoke a paired browser (default: all)
  recipes add <dir|git-url>   verify and install a recipe pack
  recipes list                installed packs and their recipes
  recipes verify [id]         offline fixture check (no network)
  recipes remove <pack>       uninstall a pack (name@sha)
  access log [-n N]           show the last N access-log lines (default 20)

state dir: $BRIDGE_STATE_DIR (default ~/.config/tomoshi)
`

func main() {
	// A pack install may clone from git: give it a deadline instead of hanging
	// the terminal on an unreachable host.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	switch args[0] {
	case "bridge":
		return bridgeCmd(args[1:])
	case "recipes":
		return recipesCmd(ctx, args[1:])
	case "access":
		return accessCmd(args[1:])
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

// stateDir resolves the same dir the server uses.
func stateDir() (string, error) {
	if dir := bridge.DefaultStateDir(); dir != "" {
		return dir, nil
	}
	return "", fmt.Errorf("cannot resolve home dir; set BRIDGE_STATE_DIR")
}

func bridgeCmd(args []string) error {
	dir, err := stateDir()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("bridge: want code, status, or unpair")
	}
	if args[0] == "unpair" {
		return unpairCmd(dir, args[1:])
	}
	info, err := bridge.ReadInfo(dir)
	if err != nil {
		return fmt.Errorf("no bridge state at %s (is the API running, or has it ever paired?): %w", dir, err)
	}
	switch args[0] {
	case "code":
		if info.Code == "" {
			return fmt.Errorf("no pairing code recorded yet")
		}
		age := time.Since(info.CodeAt).Round(time.Second)
		fmt.Printf("pairing code: %s\n", info.Code)
		if !info.CodeAt.IsZero() {
			fmt.Printf("issued:       %s (%s ago; rotates every %s)\n",
				info.CodeAt.Local().Format(time.RFC3339), age, bridge.CodeTTL)
		}
		if info.Port != "" {
			fmt.Printf("port:         %s\n", info.Port)
		}
		fmt.Println("\nEnter it in the bridge extension popup, once per browser.")
		return nil
	case "status":
		fmt.Printf("state:        %s\n", info.StatePath)
		if info.Port != "" {
			fmt.Printf("port:         %s\n", info.Port)
		}
		if len(info.Devices) == 0 {
			fmt.Println("devices:      none paired yet")
			return nil
		}
		fmt.Printf("devices:      %d\n", len(info.Devices))
		for _, d := range info.Devices {
			seen := "never"
			if !d.LastSeen.IsZero() {
				seen = time.Since(d.LastSeen).Round(time.Second).String() + " ago"
			}
			fmt.Printf("  - %s (%s), paired %s, last seen %s\n",
				d.Name, shortHash(d.TokenSHA), d.CreatedAt.Local().Format(time.RFC3339), seen)
		}
		return nil
	default:
		return fmt.Errorf("bridge: unknown subcommand %q", args[0])
	}
}

// unpairCmd revokes tokens. It edits the state file the running server
// re-reads on its next auth check, so revocation takes effect without a
// restart (and without waiting for the code to rotate).
func unpairCmd(dir string, args []string) error {
	who := "all"
	if len(args) > 0 {
		who = args[0]
	}
	st, err := bridge.NewState(dir, nil)
	if err != nil {
		return fmt.Errorf("no bridge state at %s: %w", dir, err)
	}
	// Unpair removes device hashes only; the caller's state instance is the
	// one that owns the file, so use its own writer rather than the CLI's view.
	removed := st.Unpair(who)
	if removed == 0 {
		fmt.Printf("nothing to revoke (no device named %q)\n", who)
		return nil
	}
	fmt.Printf("revoked %d device(s); the extension will need to pair again\n", removed)
	return nil
}

// shortHash previews a token hash without panicking on a short or hand-edited
// value (the README tells users to edit this file).
func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}

func recipesCmd(ctx context.Context, args []string) error {
	dir, err := stateDir()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("recipes: want add, list, verify or remove")
	}
	switch args[0] {
	case "add":
		if len(args) < 2 {
			return fmt.Errorf("recipes add: want a directory or git URL")
		}
		m, dest, err := recipe.Install(ctx, args[1], dir)
		if err != nil {
			return err
		}
		fmt.Printf("installed %s v%d -> %s\n", m.Name, m.Version, dest)
		for _, f := range m.Files {
			fmt.Printf("  %s\n", f.Path)
		}
		return nil

	case "list":
		packs, err := recipe.List(dir)
		if err != nil {
			return err
		}
		if len(packs) == 0 {
			fmt.Println("no packs installed")
			return nil
		}
		for _, p := range packs {
			fmt.Printf("%s@%s\n", p.Name, p.SHA)
			for _, id := range p.Recipes {
				fmt.Printf("  %s\n", id)
			}
		}
		return nil

	case "verify":
		reg := recipe.LoadRegistry(dir)
		for _, w := range reg.Warnings() {
			fmt.Fprintf(os.Stderr, "warning: %s\n", w)
		}
		targets := reg.All()
		if len(args) > 1 {
			r, ok := reg.ByID(args[1])
			if !ok {
				return fmt.Errorf("recipes verify: %q is not installed", args[1])
			}
			targets = []*recipe.Recipe{r}
		}
		if len(targets) == 0 {
			fmt.Println("no recipes installed; nothing to verify")
			return nil
		}
		failed := 0
		for _, r := range targets {
			// Hashes first: `verify` is the command a user runs to decide whether
			// a pack is still the pack, and a fixture edited after install would
			// otherwise re-verify green.
			if r.PackDir != "" {
				if _, err := recipe.VerifyManifest(r.PackDir); err != nil {
					failed++
					fmt.Printf("FAIL %s: %v\n", r.ID, err)
					continue
				}
			}
			results, err := recipe.VerifyFixtures(r)
			if err != nil {
				failed++
				fmt.Printf("FAIL %s: %v\n", r.ID, err)
				continue
			}
			for _, res := range results {
				if res.Pass {
					fmt.Printf("ok   %s  %s\n", r.ID, res.URL)
				} else {
					failed++
					fmt.Printf("FAIL %s  %s: %s\n", r.ID, res.URL, res.Detail)
				}
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d fixture check(s) failed", failed)
		}
		fmt.Println("all fixtures pass (offline)")
		return nil

	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("recipes remove: want a pack name (name@sha from recipes list)")
		}
		if err := recipe.Remove(dir, args[1]); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", args[1])
		return nil

	default:
		return fmt.Errorf("recipes: unknown subcommand %q", args[0])
	}
}

func accessCmd(args []string) error {
	dir, err := stateDir()
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] != "log" {
		return fmt.Errorf("access: unknown subcommand %q", args[0])
	}
	n := 20
	for i := 0; i < len(args); i++ {
		if args[i] == "-n" && i+1 < len(args) {
			fmt.Sscanf(args[i+1], "%d", &n)
		}
	}
	if n <= 0 {
		n = 20
	}

	path := filepath.Join(dir, "access.log.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("no access log yet at %s", path)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for _, line := range lines {
		var ev bridge.AuditEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			fmt.Println(line)
			continue
		}
		parts := []string{ev.TS, ev.Event}
		for _, kv := range []struct{ k, v string }{
			{"recipe", ev.Recipe}, {"lane", ev.Lane}, {"io", ev.IO},
			{"origin", ev.Origin}, {"result", ev.Result}, {"note", ev.Note},
		} {
			if kv.v != "" {
				parts = append(parts, kv.k+"="+kv.v)
			}
		}
		if ev.Actions > 0 {
			parts = append(parts, fmt.Sprintf("actions=%d", ev.Actions))
		}
		if ev.Ms > 0 {
			parts = append(parts, fmt.Sprintf("ms=%d", ev.Ms))
		}
		fmt.Println(strings.Join(parts, "  "))
	}
	return nil
}
