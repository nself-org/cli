package commands

// Purpose: P7-PROD-84 tests for `db migrate up --dry-run` without
// --migration-dir: the local path issues only SELECT statements, and the
// remote path forwards --dry-run only to a remote that proves it supports it.
// Inputs: the statement-recording fake docker (P7-PROD-77), stubbed SSH.
// Outputs: assertions over recorded statements and the exact remote argv.

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// Local default-directory dry-run: lists the pending file, and every recorded
// statement is a SELECT (no ensure*, no ledger upgrade, no BEGIN probe).
func TestDryRun_DefaultDir_LocalIssuesSelectsOnly(t *testing.T) {
	dirProject(t)
	sd := useFakeDocker(t, map[string]string{
		"q_legacy_exists": "yes\n", "q_ops_exists": "yes\n",
		"q_applied": "001_a.sql|2026-10-04T10:00:00Z\n",
	})
	cmd, _ := newDirTestCmd("up")
	_ = cmd.Flags().Set("dry-run", "true")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	calls := dockerCalls(t, sd)
	if w := writes(calls); len(w) != 0 {
		t.Fatalf("dry-run issued writes: %v\nall calls: %v", w, calls)
	}
	for _, c := range calls {
		if strings.HasPrefix(c, "CALL QUERY") && !strings.HasPrefix(c, "CALL QUERY SELECT") {
			t.Errorf("non-SELECT query during dry-run: %s", c)
		}
	}
}

// A never-migrated database: nothing is created by the dry-run.
func TestDryRun_DefaultDir_NoLedgerCreatesNothing(t *testing.T) {
	dirProject(t)
	sd := useFakeDocker(t, map[string]string{"q_legacy_exists": "no\n"})
	cmd, _ := newDirTestCmd("up")
	_ = cmd.Flags().Set("dry-run", "true")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if w := writes(dockerCalls(t, sd)); len(w) != 0 {
		t.Fatalf("dry-run issued writes: %v", w)
	}
}

// Without --dry-run the default path still applies (the guard is not a no-op).
func TestDryRun_DefaultDir_WithoutDryRunStillApplies(t *testing.T) {
	dirProject(t)
	sd := useFakeDocker(t, map[string]string{"q_legacy_exists": "yes\n", "q_ops_exists": "yes\n"})
	cmd, _ := newDirTestCmd("up")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("up: %v", err)
	}
	if len(writes(dockerCalls(t, sd))) == 0 {
		t.Fatal("plain up issued no writes")
	}
}

// Exact remote argv: dry-run forwards --dry-run (after the probe); a plain
// run forwards no --dry-run.
func TestRemoteUp_DefaultDir_DryRunArgvIsExact(t *testing.T) {
	sent, err := remoteUpWith(t, "1.5.0", "1.5.0", map[string]string{"dry-run": "true"})
	if err != nil {
		t.Fatal(err)
	}
	wantRemote(t, sent, "cd '/opt/nself' && nself 'db' 'migrate' 'up' '--dry-run'")
}

func TestRemoteUp_DefaultDir_NonDryRunArgvIsExact(t *testing.T) {
	sent, err := remoteUpWith(t, "1.5.0", "1.5.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	wantRemote(t, sent, "cd '/opt/nself' && nself 'db' 'migrate' 'up'")
}

// Refusals happen before any command that could apply is sent: the log holds
// at most the version probe, never the `cd ... && nself ...` command.
func TestRemoteUp_DryRunRefusals(t *testing.T) {
	cases := []struct {
		name          string
		local, remote string
		flags         map[string]string
		wantErr       string
		wantProbe     bool
	}{
		{"older remote", "1.5.0", "1.4.9", map[string]string{"dry-run": "true"}, "remote nself is v1.4.9", true},
		{"older remote under --allow-version-drift", "1.5.0", "1.4.9", map[string]string{"dry-run": "true", "allow-version-drift": "true"}, "--allow-version-drift does not apply to --dry-run", true},
		{"older remote, dir dry-run, drift flag", "1.5.0", "1.4.9", map[string]string{"migration-dir": "m", "dry-run": "true", "allow-version-drift": "true"}, "remote nself is v1.4.9", true},
		{"newer remote", "1.5.0", "1.6.0", map[string]string{"dry-run": "true"}, "remote nself is v1.6.0", true},
		{"dev build cannot prove", "dev", "1.5.0", map[string]string{"dry-run": "true"}, "no release version", false},
		{"dev build under drift flag", "dev", "1.5.0", map[string]string{"dry-run": "true", "allow-version-drift": "true"}, "no release version", false},
		{"unparseable remote version", "1.5.0", "garbage", map[string]string{"dry-run": "true", "allow-version-drift": "true"}, "no parseable version", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sent, err := remoteUpWith(t, c.local, c.remote, c.flags)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("want refusal containing %q, got %v", c.wantErr, err)
			}
			for _, s := range sent {
				if s != "nself --version" {
					t.Fatalf("a command was sent despite the refusal: %q", sent)
				}
			}
			if c.wantProbe != (len(sent) == 1) {
				t.Fatalf("probe sent = %v, want %v (log %q)", len(sent) == 1, c.wantProbe, sent)
			}
		})
	}
}

// --allow-version-drift with a remote that proves support (same version) runs
// the dry-run; it only ever relaxes non-dry-run commands.
func TestRemoteUp_DryRunUnderDriftFlagProceedsWhenRemoteProves(t *testing.T) {
	sent, err := remoteUpWith(t, "1.5.0", "1.5.0", map[string]string{"dry-run": "true", "allow-version-drift": "true"})
	if err != nil {
		t.Fatal(err)
	}
	wantRemote(t, sent, "cd '/opt/nself' && nself 'db' 'migrate' 'up' '--dry-run'")
}

// Non-dry-run: --allow-version-drift still skips the probe (unchanged).
func TestRemoteUp_NonDryRunDriftFlagStillSkipsProbe(t *testing.T) {
	sent, err := remoteUpWith(t, "1.5.0", "1.4.9", map[string]string{"allow-version-drift": "true"})
	want := "cd '/opt/nself' && nself 'db' 'migrate' 'up'"
	if err != nil || len(sent) != 1 || sent[0] != want {
		t.Fatalf("sent %q, err %v; want exactly [%q]", sent, err, want)
	}
}

// A failing probe is a refusal, never a pass, and no apply-capable command goes out.
func TestRemoteUp_DryRunProbeFailureRefuses(t *testing.T) {
	withLocalVersion(t, "1.5.0")
	writeRemoteInventory(t)
	var sent []string
	withStubbedSSH(t, func(ctx context.Context, sshArgs []string) (string, error) {
		sent = append(sent, sshArgs[len(sshArgs)-1])
		return "ssh: connection refused", fmt.Errorf("exit status 255")
	})
	cmd := newDBRemoteTestCmd()
	cmd.Flags().String("migration-dir", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	_ = cmd.Flags().Set("env", "staging")
	_ = cmd.Flags().Set("dry-run", "true")
	_ = cmd.Flags().Set("allow-version-drift", "true")
	err := runDBMigrateUp(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "could not read the remote nself version") {
		t.Fatalf("want probe-failure refusal, got %v", err)
	}
	if len(sent) != 1 || sent[0] != "nself --version" {
		t.Fatalf("ssh log = %q, want only the probe", sent)
	}
}

// hasDryRunArg is exact: a directory named like the flag value is not a flag.
func TestHasDryRunArg(t *testing.T) {
	if !hasDryRunArg([]string{"db", "migrate", "up", "--dry-run"}) {
		t.Error("--dry-run not detected")
	}
	if hasDryRunArg([]string{"db", "migrate", "up", "--migration-dir", "--dry-run-x"}) {
		t.Error("lookalike detected as --dry-run")
	}
}
