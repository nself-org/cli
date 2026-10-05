package commands

// Purpose: P7-PROD-84 tests for `db migrate up --dry-run` without
// --migration-dir: the local path issues only SELECT statements, and the
// remote path forwards --dry-run only to a remote that proves it supports it.
// Inputs: the statement-recording fake docker (P7-PROD-77), stubbed SSH.
// Outputs: assertions over recorded statements and the exact remote argv.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/version"
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

// Exact remote argv: dry-run forwards --dry-run (after the capability probe);
// a plain run forwards no --dry-run and sends no capability probe.
func TestRemoteUp_DefaultDir_DryRunArgvIsExact(t *testing.T) {
	sent, err := remoteUpWith(t, "1.4.12", capableRemote(), map[string]string{"dry-run": "true"})
	if err != nil {
		t.Fatal(err)
	}
	wantRemote(t, sent, probeCaps, "cd '/opt/nself' && nself 'db' 'migrate' 'up' '--dry-run'")
}

func TestRemoteUp_DefaultDir_NonDryRunArgvIsExact(t *testing.T) {
	sent, err := remoteUpWith(t, "1.4.12", capableRemote(), nil)
	if err != nil {
		t.Fatal(err)
	}
	wantRemote(t, sent, probeVersion, "cd '/opt/nself' && nself 'db' 'migrate' 'up'")
}

// Refusals happen before any command that could apply is sent: the log holds
// only the capability probe, never the `cd ... && nself ...` command. The
// released v1.4.12 applies on a dry-run and reports the same version number
// as a source build, so a matching version must NOT be accepted.
func TestRemoteUp_DryRunRefusals(t *testing.T) {
	cases := []struct {
		name    string
		local   string
		remote  fakeRemote
		flags   map[string]string
		wantErr string
	}{
		{"released 1.4.12 without the capability, same version as local", "1.4.12",
			fakeRemote{Version: "1.4.12", VersionJSON: `{"version":"1.4.12","commit":"abc"}`},
			map[string]string{"dry-run": "true"}, "does not advertise"},
		{"1.4.12 without the capability under --allow-version-drift", "1.4.12",
			fakeRemote{Version: "1.4.12", VersionJSON: `{"version":"1.4.12","capabilities":[]}`},
			map[string]string{"dry-run": "true", "allow-version-drift": "true"}, "--allow-version-drift does not apply to --dry-run"},
		{"1.4.12 without the capability, dir dry-run", "1.4.12",
			fakeRemote{Version: "1.4.12", VersionJSON: `{"version":"1.4.12"}`},
			map[string]string{"migration-dir": "m", "dry-run": "true"}, "does not advertise"},
		{"other capabilities only", "1.4.12",
			fakeRemote{VersionJSON: capsJSON("9.9.9", "something-else")},
			map[string]string{"dry-run": "true"}, "does not advertise"},
		{"lookalike capability name", "1.4.12",
			fakeRemote{VersionJSON: capsJSON("9.9.9", "db-dry-run-safe-ish", "DB-DRY-RUN-SAFE")},
			map[string]string{"dry-run": "true"}, "does not advertise"},
		{"garbage output", "1.4.12", fakeRemote{VersionJSON: "bash: nself: command not found"},
			map[string]string{"dry-run": "true", "allow-version-drift": "true"}, "not JSON"},
		{"truncated JSON", "1.4.12", fakeRemote{VersionJSON: `{"capabilities":["db-dry-run-safe"`},
			map[string]string{"dry-run": "true"}, "not JSON"},
		{"wrong JSON types", "1.4.12", fakeRemote{VersionJSON: `{"capabilities":"db-dry-run-safe"}`},
			map[string]string{"dry-run": "true"}, "not valid version JSON"},
		{"empty output", "1.4.12", fakeRemote{VersionJSON: ""},
			map[string]string{"dry-run": "true"}, "not JSON"},
		{"probe error", "1.4.12", fakeRemote{VersionJSON: "Connection refused", ProbeErr: fmt.Errorf("exit status 255")},
			map[string]string{"dry-run": "true", "allow-version-drift": "true"}, "could not read the remote capabilities"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sent, err := remoteUpWith(t, c.local, c.remote, c.flags)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("want refusal containing %q, got %v", c.wantErr, err)
			}
			if len(sent) != 1 || sent[0] != probeCaps {
				t.Fatalf("ssh log = %q, want only the capability probe", sent)
			}
		})
	}
}

// A timed-out probe is a refusal and nothing else is sent.
func TestRemoteUp_DryRunProbeTimeoutRefuses(t *testing.T) {
	orig := remoteProbeTimeout
	remoteProbeTimeout = 50 * time.Millisecond
	t.Cleanup(func() { remoteProbeTimeout = orig })
	withLocalVersion(t, "1.4.12")
	writeRemoteInventory(t)
	var sent []string
	withStubbedSSH(t, func(ctx context.Context, sshArgs []string) (string, error) {
		sent = append(sent, sshArgs[len(sshArgs)-1])
		<-ctx.Done() // a hung remote: only the probe deadline ends the call
		return "", ctx.Err()
	})
	cmd := newDBRemoteTestCmd()
	cmd.Flags().String("migration-dir", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	_ = cmd.Flags().Set("env", "staging")
	_ = cmd.Flags().Set("dry-run", "true")
	err := runDBMigrateUp(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("want timeout refusal, got %v", err)
	}
	if len(sent) != 1 || sent[0] != probeCaps {
		t.Fatalf("ssh log = %q, want only the probe", sent)
	}
}

// A remote that advertises the capability is accepted whatever its version
// says, including under --allow-version-drift and for a dev local build; the
// capability is the proof, the version is not consulted.
func TestRemoteUp_DryRunAcceptedWhenRemoteAdvertises(t *testing.T) {
	for _, tc := range []struct {
		local string
		flags map[string]string
	}{
		{"1.4.12", map[string]string{"dry-run": "true"}},
		{"1.4.12", map[string]string{"dry-run": "true", "allow-version-drift": "true"}},
		{"dev", map[string]string{"dry-run": "true"}},
	} {
		sent, err := remoteUpWith(t, tc.local, fakeRemote{VersionJSON: "warn: noise\n" + capsJSON("1.9.0", version.CapDBDryRunSafe, "x")}, tc.flags)
		if err != nil {
			t.Fatalf("%v: %v", tc, err)
		}
		wantRemote(t, sent, probeCaps, "cd '/opt/nself' && nself 'db' 'migrate' 'up' '--dry-run'")
	}
}

// Non-dry-run: --allow-version-drift still skips the probe (unchanged).
func TestRemoteUp_NonDryRunDriftFlagStillSkipsProbe(t *testing.T) {
	sent, err := remoteUpWith(t, "1.4.12", fakeRemote{Version: "1.4.9"}, map[string]string{"allow-version-drift": "true"})
	want := "cd '/opt/nself' && nself 'db' 'migrate' 'up'"
	if err != nil || len(sent) != 1 || sent[0] != want {
		t.Fatalf("sent %q, err %v; want exactly [%q]", sent, err, want)
	}
}

// This build advertises the capability, and `version --json` carries it, so
// the probe a same-build remote receives passes the check.
func TestVersionJSON_AdvertisesDryRunCapability(t *testing.T) {
	caps := version.Capabilities()
	if !slices.Contains(caps, version.CapDBDryRunSafe) {
		t.Fatalf("Capabilities() = %v, missing %s", caps, version.CapDBDryRunSafe)
	}
	got, err := parseRemoteCapabilities(capsJSON(version.GetVersion(), caps...))
	if err != nil || !slices.Contains(got, version.CapDBDryRunSafe) {
		t.Fatalf("round trip = %v, %v", got, err)
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
