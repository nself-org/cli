package commands

// root_repo_scoped_test.go — Guards the repo-scoped command exemption.
//
// Purpose: `nself ci` operates on a source checkout, not a running stack, so it
//          must be exempt from the monorepo redirect and the source-repo guard.
//          Lifecycle commands must NOT be exempt — that redirect is why `stop`,
//          `logs` and `status` work from a monorepo root.
// Inputs:  command names.
// Outputs: assertions on isRepoScopedCommand / isSourceSafeCommand.
// Constraints: pure predicate tests; no filesystem, no chdir, no Docker.
//
// The bug this pins: PersistentPreRunE chdir'd into the detected backend/
// before RunE, so `nself ci --check .` resolved "." against backend/ rather
// than the directory the user typed it in. From inside ntask it announced
// "Using backend as project root" and failed, because backend/ has no manifest
// while the repo root has package.json and pnpm-workspace.yaml. It overrode an
// explicitly passed path, which is what made it a correctness bug.

import (
	"github.com/spf13/cobra"
	"testing"
)

// TestRepoScopedCommands_ExemptFromRedirect pins which commands skip the
// monorepo chdir. Adding a name here is a deliberate act: it means the command
// takes a repo path and must see the user's cwd.
func TestRepoScopedCommands_ExemptFromRedirect(t *testing.T) {
	t.Parallel()

	if !isRepoScopedCommand("ci") {
		t.Error("ci must be repo-scoped: the monorepo redirect breaks its [repo-root] argument")
	}

	// Lifecycle commands rely on the redirect to work from a monorepo root.
	// If any of these becomes exempt, `nself stop` from the repo root silently
	// targets the wrong directory.
	for _, name := range []string{"start", "stop", "restart", "status", "logs", "build", "exec", "reset"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if isRepoScopedCommand(name) {
				t.Errorf("%q must NOT be repo-scoped — it needs the monorepo redirect", name)
			}
		})
	}
}

// TestRepoScopedImpliesSourceSafe covers the other half of the exemption:
// running `nself ci` inside a checkout is the entire point, so the
// source-repo guard must not block it.
func TestRepoScopedImpliesSourceSafe(t *testing.T) {
	t.Parallel()

	if !isSourceSafeCommand("ci") {
		t.Error("ci must be source-safe: refusing to run inside a checkout defeats the gate")
	}

	// The pre-existing safe list must survive the change.
	for _, name := range []string{"help", "version", "completion", "update", "upgrade", "doctor", "nself"} {
		if !isSourceSafeCommand(name) {
			t.Errorf("%q lost its source-safe exemption", name)
		}
	}

	// And genuinely unsafe commands must still be blocked in a source repo.
	for _, name := range []string{"start", "build", "reset"} {
		if isSourceSafeCommand(name) {
			t.Errorf("%q must NOT be source-safe — it would run against the nself checkout", name)
		}
	}
}

func TestRepoScopedInvocation(t *testing.T) {
	for _, tc := range []struct {
		path []string
		want bool
	}{
		{[]string{"nself", "ci"}, true},
		{[]string{"nself", "config", "secrets", "resolve"}, true},
		{[]string{"nself", "config", "secrets", "recipients"}, true},
		{[]string{"nself", "config", "secrets", "recipients", "list"}, true},
		{[]string{"nself", "config", "secrets", "get"}, false},
		{[]string{"nself", "plugin", "resolve"}, false},
		{[]string{"nself", "start"}, false},
	} {
		t.Run(tc.path[len(tc.path)-1], func(t *testing.T) {
			cmd := &cobra.Command{Use: tc.path[0]}
			for _, name := range tc.path[1:] {
				child := &cobra.Command{Use: name}
				cmd.AddCommand(child)
				cmd = child
			}
			if got := isRepoScopedInvocation(cmd); got != tc.want {
				t.Errorf("%v: got %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
