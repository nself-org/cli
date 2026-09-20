package commands

// Purpose: every parent command must reject an unknown subcommand instead of
// printing help and exiting 0.
//
// Inputs:  the assembled RootCmd tree.
// Outputs: assertions on each parent's Args validator.
// Constraints: regression guard for the whole tree, not one command.
//
// A cobra parent whose RunE is `return cmd.Help()` and which declares no Args
// validator swallows unknown subcommands: cobra finds no matching child, falls
// back to the parent, runs RunE, prints the Long description on stdout and
// exits 0. The caller sees success and captures help prose as data.
//
// That is not hypothetical. Golden-path step 13 ran
//
//	base_url=$(nself env get NSELF_API_URL 2>/dev/null || echo "http://localhost:8080")
//
// There is no `env get`. Exit status was 0, so the `||` fallback never fired,
// base_url became the multi-line help text, and every probe curled garbage —
// a silent wrong answer rather than a loud failure (fixed in e540c85e).
//
// TestEveryParentRejectsUnknownSubcommand walks the real tree rather than a
// hand-maintained list, so a parent added later is covered the day it lands.

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// parentsExemptFromNoArgs are the parents that legitimately accept a positional
// argument. Each still fails loudly on an unknown subcommand — by its own
// means, asserted in TestExemptParentsStillRejectUnknownSubcommand — so the
// exemption is a different mechanism, never a missing check.
var parentsExemptFromNoArgs = map[string]string{
	"nself": "Execute() intercepts an unknown first arg before cobra sees it and " +
		"routes it to the plugin proxy (`nself <plugin> <action>`). RootCmd.Args " +
		"is unreachable on that path; cobra's own legacyArgs covers the rest.",
	"nself plugin": "proxies unknown subcommands to the matching plugin binary " +
		"(`nself plugin <name> <action>` -> `nself-<name> <action>`). NoArgs here " +
		"would delete a documented feature.",
	"nself bundle": "its RunE already returns an explicit error naming the bad " +
		"subcommand, which is friendlier than cobra's generic message.",
}

// runnableParents are parents that are also real leaf commands: their RunE does
// actual work (opens the admin UI, prints health, checks for updates) rather
// than falling through to help. They ignore surplus positional args instead of
// printing help and exiting 0, so they are not instances of the bug this file
// guards. Tightening them is a deliberate behavior change, tracked separately.
var runnableParents = map[string]bool{
	"nself account": true,
	"nself admin":   true,
	"nself ci":      true,
	"nself ci eval": true,
	"nself health":  true,
	"nself migrate": true,
	"nself trust":   true,
	"nself update":  true,
}

// argTakingParents document a positional argument in their own Use string, so
// NoArgs would break their documented calling convention. They are parents and
// leaves at once: `nself deploy staging` is the command, `nself deploy status`
// is a subcommand. Their existing validator already bounds the arity.
var argTakingParents = map[string]string{
	"nself db backup": "`backup [file]` — optional dump path, MaximumNArgs(1)",
	"nself db seed":   "`seed` takes an optional seed name, MaximumNArgs(1)",
	"nself deploy":    "`deploy [target]` — target may be positional or --env, MaximumNArgs(1)",
	"nself promote":   "`promote <source> <target>` — ExactArgs(2)",
}

func parentCommands(t *testing.T) []*cobra.Command {
	t.Helper()
	ApplyCommandGroups()
	var out []*cobra.Command
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.HasSubCommands() {
			out = append(out, c)
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd)
	return out
}

func TestEveryParentRejectsUnknownSubcommand(t *testing.T) {
	for _, cmd := range parentCommands(t) {
		path := cmd.CommandPath()
		if _, exempt := parentsExemptFromNoArgs[path]; exempt {
			continue
		}
		if runnableParents[path] {
			continue
		}
		if _, takesArgs := argTakingParents[path]; takesArgs {
			continue
		}
		t.Run(strings.ReplaceAll(path, " ", "_"), func(t *testing.T) {
			if cmd.Args == nil {
				t.Fatalf("%s has subcommands but no Args validator: an unknown "+
					"subcommand will print help and exit 0. Add Args: cobra.NoArgs.", path)
			}
			for _, argv := range [][]string{
				{"bogus-subcommand"},
				{"get", "NSELF_API_URL"},
				{"set", "FOO", "bar"},
			} {
				if err := cmd.Args(cmd, argv); err == nil {
					t.Errorf("%s %v must be rejected, got nil error", path, argv)
				}
			}
		})
	}
}

func TestEveryParentStillPrintsHelpWhenBare(t *testing.T) {
	// NoArgs permits zero args, so `nself db` must keep printing help.
	for _, cmd := range parentCommands(t) {
		if cmd.Args == nil {
			continue
		}
		path := cmd.CommandPath()
		if _, takesArgs := argTakingParents[path]; takesArgs {
			// e.g. promote requires ExactArgs(2); bare invocation is an error
			// by design, not a regression.
			continue
		}
		t.Run(strings.ReplaceAll(path, " ", "_"), func(t *testing.T) {
			if err := cmd.Args(cmd, []string{}); err != nil {
				t.Errorf("bare `%s` must be allowed, got %v", path, err)
			}
		})
	}
}

func TestExemptParentsStillRejectUnknownSubcommand(t *testing.T) {
	// The exemptions above are only acceptable while the commands keep failing
	// loudly by their own mechanism. bundle and plugin do it in RunE, so drive
	// RunE directly. (plugin's proxy only stats the plugin bin dir for a
	// command that cannot exist; it execs nothing.)
	for _, tc := range []struct {
		path string
		cmd  *cobra.Command
	}{
		{"nself bundle", bundleCmd},
		{"nself plugin", pluginCmd},
	} {
		t.Run(strings.ReplaceAll(tc.path, " ", "_"), func(t *testing.T) {
			argv := []string{"bogus-subcommand-that-cannot-exist"}
			if tc.cmd.Args != nil {
				if err := tc.cmd.Args(tc.cmd, argv); err != nil {
					return // rejected by a validator; equally fine
				}
			}
			if err := tc.cmd.RunE(tc.cmd, argv); err == nil {
				t.Errorf("%s %v must fail, got nil error — the NoArgs exemption "+
					"in parentsExemptFromNoArgs is no longer justified", tc.path, argv)
			}
		})
	}

	// RootCmd's guard lives in Execute(), not Args. Assert the precondition that
	// makes cobra's own legacyArgs reject `nself bogus`: root has subcommands.
	if !RootCmd.HasSubCommands() {
		t.Error("RootCmd has no subcommands; cobra's legacyArgs no longer rejects unknown commands")
	}
}

func TestExemptionListsOnlyNameRealCommands(t *testing.T) {
	// A stale exemption silently re-opens the hole it documents.
	found := map[string]bool{}
	for _, cmd := range parentCommands(t) {
		found[cmd.CommandPath()] = true
	}
	for path := range parentsExemptFromNoArgs {
		if !found[path] {
			t.Errorf("parentsExemptFromNoArgs names %q, which is not a parent command anymore", path)
		}
	}
	for path := range runnableParents {
		if !found[path] {
			t.Errorf("runnableParents names %q, which is not a parent command anymore", path)
		}
	}
	for path := range argTakingParents {
		if !found[path] {
			t.Errorf("argTakingParents names %q, which is not a parent command anymore", path)
		}
	}
}
