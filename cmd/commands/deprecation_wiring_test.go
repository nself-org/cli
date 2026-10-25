package commands

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/deprecation"
	"github.com/spf13/cobra"
)

// captureStderr runs fn with os.Stderr replaced by a pipe and returns what was
// written. Used because the deprecation warning goes to the real os.Stderr, not
// to a cobra-managed writer.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()

	fn()

	os.Stderr = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// runFromScratchDir points the test at an empty temp directory before it
// executes a real command.
//
// These tests drive RootCmd.Execute for commands like `start`, and they have to
// disable the source-repo guard to do it. That combination is dangerous: with
// the guard off and the working directory inside cmd/commands, a command that
// finds a project marker there will happily write generated files into the
// source tree. That is exactly how the tracked cmd/commands/.env.example
// fixture was rewritten during CLI-R03 and swept into a commit. Chdir'ing to a
// temp dir first makes the guard irrelevant rather than merely bypassed.
func runFromScratchDir(t *testing.T) {
	t.Helper()
	t.Setenv("NSELF_ALLOW_SOURCE_DIR", "1")
	t.Chdir(t.TempDir())
}

// TestDeprecatedAliasWarns is the regression guard for CLI-R03's second defect.
// `up` is a cobra alias of startCmd, so cmd.CommandPath() reports "nself start".
// Keying the registry lookup on CommandPath meant the warning could never fire
// for an aliased spelling — the exact mechanism every CLI-R09/R19 rename relies
// on. invokedCommandPath keys on what the user actually typed instead.
func TestDeprecatedAliasWarns(t *testing.T) {
	runFromScratchDir(t)

	out := captureStderr(t, func() {
		RootCmd.SetArgs([]string{"up"})
		_ = RootCmd.Execute()
	})

	if !strings.Contains(out, "[DEPRECATED] 'nself up'") {
		t.Fatalf("expected a deprecation warning for the `up` alias, got: %q", out)
	}
	if !strings.Contains(out, "nself start") {
		t.Fatalf("warning did not name the replacement: %q", out)
	}
}

// TestCanonicalSpellingDoesNotWarn is the other half: warning on the canonical
// name would make every correct `nself start` noisy.
func TestCanonicalSpellingDoesNotWarn(t *testing.T) {
	runFromScratchDir(t)

	out := captureStderr(t, func() {
		RootCmd.SetArgs([]string{"start"})
		_ = RootCmd.Execute()
	})

	if strings.Contains(out, "[DEPRECATED]") {
		t.Fatalf("canonical `nself start` emitted a deprecation warning: %q", out)
	}
}

// TestNoDeprecationWarningsFlagSuppresses verifies the scripted-use escape hatch.
func TestNoDeprecationWarningsFlagSuppresses(t *testing.T) {
	runFromScratchDir(t)

	out := captureStderr(t, func() {
		RootCmd.SetArgs([]string{"up", "--no-deprecation-warnings"})
		_ = RootCmd.Execute()
	})

	if strings.Contains(out, "[DEPRECATED]") {
		t.Fatalf("--no-deprecation-warnings did not suppress the warning: %q", out)
	}
}

// TestEveryDeprecatedCommandEntryIsReachable stops the registry filling up with
// entries that never produce a message.
//
// There are exactly three ways an entry can surface:
//   - the name is a registered command or an alias of one, handled by
//     PersistentPreRunE via invokedCommandPath (CLI-R03);
//   - the name is a retired top-level spelling, rewritten onto its new home by
//     legacy_spellings.go (CLI-R09);
//   - the name is no longer registered at all because the command moved out to
//     a plugin, handled by warnRelocatedCommand on the proxy path (CLI-R11).
//
// A hidden command is a fourth, legitimate shape: deprecated but still working,
// warning on its own canonical name (CLI-R19's `uninstall`).
//
// The third case is why there is no phase-3 exemption here. An earlier version
// exempted phase 3 on the grounds that a removed command is unreachable by
// definition — but that accepted a decorative entry that told the user nothing.
// Wiring the proxy path into the registry made it reachable for real, so the
// test can demand that every entry works rather than excusing the ones that do
// not.
func TestEveryDeprecatedCommandEntryIsReachable(t *testing.T) {
	reg, err := deprecation.LoadEmbeddedRegistry()
	if err != nil {
		t.Fatalf("LoadEmbeddedRegistry: %v", err)
	}

	registered := map[string]bool{}
	hidden := map[string]bool{}
	aliased := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		registered[c.CommandPath()] = true
		if c.Hidden {
			hidden[c.CommandPath()] = true
		}
		prefix := "nself"
		if p := c.Parent(); p != nil {
			prefix = p.CommandPath()
		}
		for _, a := range c.Aliases {
			aliased[prefix+" "+a] = true
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd)

	var noisy []string
	for _, name := range reg.Names() {
		item, _ := reg.Lookup(name)
		if item.Type != deprecation.TypeCommand {
			continue
		}

		// An entry naming a live command by its CANONICAL name would fire on
		// every correct invocation. That is the failure worth catching: an
		// unregistered name is handled by the proxy path, and an alias or a
		// legacy spelling is handled by its own mechanism.
		isTopLevelLegacy := false
		if rest, ok := strings.CutPrefix(name, "nself "); ok {
			_, isTopLevelLegacy = legacySpellings[rest]
		}
		// A hidden command is deprecated-but-working (CLI-R19 keeps `uninstall`
		// this way so existing scripts behave identically). Warning on its
		// canonical name is the whole point, so it is not noise.
		if registered[name] && !hidden[name] && !aliased[name] && !isTopLevelLegacy {
			noisy = append(noisy, name)
		}
	}

	if len(noisy) > 0 {
		t.Fatalf("deprecation registry names %d live command(s) by their canonical spelling, "+
			"which would warn on every correct invocation:\n  %s",
			len(noisy), strings.Join(noisy, "\n  "))
	}
}

// TestRelocatedCommandEntriesAreWiredToTheProxy proves the third surfacing
// route actually works: a registry entry for a command that has left core must
// produce its message through warnRelocatedCommand, not sit there as
// documentation.
func TestRelocatedCommandEntriesAreWiredToTheProxy(t *testing.T) {
	reg, err := deprecation.LoadEmbeddedRegistry()
	if err != nil {
		t.Fatalf("LoadEmbeddedRegistry: %v", err)
	}

	registered := map[string]bool{}
	for _, c := range RootCmd.Commands() {
		registered["nself "+c.Name()] = true
		for _, a := range c.Aliases {
			registered["nself "+a] = true
		}
	}

	checked := 0
	for _, name := range reg.Names() {
		item, _ := reg.Lookup(name)
		if item.Type != deprecation.TypeCommand || registered[name] {
			continue
		}
		bare, ok := strings.CutPrefix(name, "nself ")
		if !ok {
			continue
		}
		if _, isLegacy := legacySpellings[bare]; isLegacy {
			continue
		}

		out := captureStderr(t, func() { warnRelocatedCommand(bare) })
		if !strings.Contains(out, "[DEPRECATED]") {
			t.Errorf("relocated command %q produces no message on the proxy path", name)
			continue
		}
		if item.Replacement != "" && !strings.Contains(out, item.Replacement) {
			t.Errorf("message for %q does not name its replacement %q: %s", name, item.Replacement, out)
		}
		checked++
	}

	if checked == 0 {
		t.Skip("no relocated commands in the registry yet")
	}
}

// TestInstalledBinaryEmitsDeprecationWarning is the CLI-R03 end-to-end proof:
// build the CLI, put it in an otherwise-empty directory (no source tree, no
// registry.yaml on disk anywhere near it) and confirm the warning still fires.
// This is exactly the situation — `make install`, Homebrew, a release tarball —
// in which the old path-based loader silently produced no warnings at all.
func TestInstalledBinaryEmitsDeprecationWarning(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary; skipped under -short")
	}
	if runtime.GOOS == "windows" {
		t.Skip("binary staging in this test is POSIX-only")
	}

	// pilotBinary is one shared build of ./cmd/nself in a directory of its own,
	// with no source tree next to it (D-0735: no second compile and link).
	binPath := pilotBinary(t)

	// Run from a *different* empty directory so neither the binary's own
	// directory nor the working directory contains a source tree.
	runDir := t.TempDir()
	run := exec.Command(binPath, "up")
	run.Dir = runDir
	// Explicitly clear the override so only the embedded copy can be the source.
	run.Env = append(os.Environ(), deprecation.RegistryPathEnv+"=")

	var stderr bytes.Buffer
	run.Stderr = &stderr
	_ = run.Run() // a non-zero exit is expected: there is no project here.

	if !strings.Contains(stderr.String(), "[DEPRECATED] 'nself up'") {
		t.Fatalf("installed binary in an empty dir emitted no deprecation warning.\nstderr:\n%s", stderr.String())
	}
}

func findRepoRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// TestEveryLegacySpellingResolves checks the other direction of the CLI-R09
// rewrite: each retired name must land on a command that actually exists, and
// must carry a deprecation entry so the user is told where it went. A typo in
// either table would otherwise produce a command that silently does nothing or
// a rename nobody is told about.
//
// One shape is not a live cobra command: CLI-R11 can extract the *target* of
// a rewrite out of core (e.g. "ollama" rewrites to ["model", "ollama"], and
// "model" itself later left core for the model plugin). RootCmd.Find then
// correctly reports the first segment as unresolvable — root.go's Execute()
// proxies it to the plugin binary at runtime instead of dispatching to a
// cobra command, and warnRelocatedCommand (not tested here) covers that
// path's own warning. This test still requires that unresolvable case to
// name a top-level segment with its own deprecation entry, so a genuine typo
// in the canonical path (rather than an intentional extraction) still fails.
func TestEveryLegacySpellingResolves(t *testing.T) {
	reg, err := deprecation.LoadEmbeddedRegistry()
	if err != nil {
		t.Fatalf("LoadEmbeddedRegistry: %v", err)
	}

	for name, entry := range legacySpellings {
		if len(entry.canonical) == 0 {
			t.Errorf("%q maps to an empty canonical path", name)
			continue
		}

		want := "nself " + strings.Join(entry.canonical, " ")

		target, _, findErr := RootCmd.Find(entry.canonical)
		if findErr != nil {
			// The first segment may itself be a relocated command (proxied
			// to a plugin at runtime rather than resolved by cobra). That is
			// only valid if it carries its own deprecation entry — otherwise
			// this is an ordinary broken mapping.
			head := "nself " + entry.canonical[0]
			if _, headOK := reg.Lookup(head); !headOK {
				t.Errorf("%q maps to %v, which cobra cannot resolve (%v), and %q has no deprecation entry explaining why",
					name, entry.canonical, findErr, head)
				continue
			}
		} else if target.CommandPath() != want {
			t.Errorf("%q maps to %v but cobra resolved it to %q", name, entry.canonical, target.CommandPath())
		}

		item, ok := reg.Lookup("nself " + name)
		if !ok {
			t.Errorf("%q has no deprecation registry entry — users get no warning", name)
			continue
		}
		if item.Replacement != want {
			t.Errorf("%q registry replacement is %q; the rewrite sends it to %q",
				name, item.Replacement, want)
		}
	}
}

// TestLegacySpellingsAreNotRegisteredCommands guards the ordering requirement:
// a retired name must NOT still be a registered top-level command, or the
// rewrite would shadow a real command instead of redirecting a dead one.
func TestLegacySpellingsAreNotRegisteredCommands(t *testing.T) {
	for _, c := range RootCmd.Commands() {
		if _, ok := legacySpellings[c.Name()]; ok {
			t.Errorf("%q is both a registered top-level command and a legacy spelling", c.Name())
		}
	}
}
