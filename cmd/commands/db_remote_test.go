package commands

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/version"
	"github.com/spf13/cobra"
)

// newDBRemoteTestCmd returns a minimal cobra.Command with --env/--server
// registered via addDBRemoteFlags, mirroring how dbMigrateUpCmd etc. are
// wired in db.go's init().
func newDBRemoteTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "up"}
	addDBRemoteFlags(cmd)
	cmd.SetContext(context.Background())
	return cmd
}

// TestAddDBRemoteFlags_Registered verifies --env and --server are both
// registered by addDBRemoteFlags (gap #9: these commands had zero flags
// before this change).
func TestAddDBRemoteFlags_Registered(t *testing.T) {
	cmd := newDBRemoteTestCmd()
	if cmd.Flags().Lookup("env") == nil {
		t.Error("expected --env flag to be registered")
	}
	if cmd.Flags().Lookup("server") == nil {
		t.Error("expected --server flag to be registered")
	}
}

// TestResolveDBRemoteTarget_DefaultsLocal verifies that omitting --env/--server
// entirely resolves to Local=true, preserving today's default behavior for
// every existing caller that doesn't pass these new flags.
func TestResolveDBRemoteTarget_DefaultsLocal(t *testing.T) {
	cmd := newDBRemoteTestCmd()
	target, err := resolveDBRemoteTarget(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !target.Local {
		t.Error("expected Local=true when --env/--server are unset")
	}
}

// TestResolveDBRemoteTarget_EnvLocal verifies --env=local also resolves to
// Local=true without touching control-plane inventory.
func TestResolveDBRemoteTarget_EnvLocal(t *testing.T) {
	cmd := newDBRemoteTestCmd()
	if err := cmd.Flags().Set("env", "local"); err != nil {
		t.Fatalf("set --env: %v", err)
	}
	target, err := resolveDBRemoteTarget(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !target.Local {
		t.Error("expected Local=true for --env=local")
	}
}

// TestResolveDBRemoteTarget_UnknownEnvironment verifies that requesting a
// remote environment with no matching control-plane inventory entry and no
// legacy env var returns a descriptive error rather than silently running
// locally (which would silently apply migrations to the wrong database).
func TestResolveDBRemoteTarget_UnknownEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	cmd := newDBRemoteTestCmd()
	if err := cmd.Flags().Set("env", "staging"); err != nil {
		t.Fatalf("set --env: %v", err)
	}
	_, err := resolveDBRemoteTarget(cmd)
	if err == nil {
		t.Fatal("expected an error when --env=staging has no configured inventory/host")
	}
	if !strings.Contains(err.Error(), "staging") {
		t.Errorf("expected error to mention 'staging', got: %v", err)
	}
}

// TestResolveDBRemoteTarget_RemoteServer verifies that a control-plane
// inventory entry with a real Host resolves to a non-local target carrying
// the expected SSH target and remote path.
func TestResolveDBRemoteTarget_RemoteServer(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	inv := &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"local": {Name: "local", Kind: "local", Servers: []controlplane.Server{{Name: "local-app", Role: controlplane.RoleApp}}},
			"staging": {
				Name: "staging",
				Kind: "remote",
				Servers: []controlplane.Server{
					{
						Name:       "staging-app",
						Role:       controlplane.RoleApp,
						Host:       "deploy@staging.example.com",
						RemotePath: "/opt/nself",
						Primary:    true,
					},
				},
			},
		},
	}
	if err := controlplane.Write(dir, inv); err != nil {
		t.Fatalf("write inventory: %v", err)
	}

	cmd := newDBRemoteTestCmd()
	if err := cmd.Flags().Set("env", "staging"); err != nil {
		t.Fatalf("set --env: %v", err)
	}
	target, err := resolveDBRemoteTarget(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Local {
		t.Fatal("expected Local=false for a staging environment with a real Host")
	}
	if target.SSHTarget != "deploy@staging.example.com" {
		t.Errorf("SSHTarget: got %q, want %q", target.SSHTarget, "deploy@staging.example.com")
	}
	if target.RemotePath != "/opt/nself" {
		t.Errorf("RemotePath: got %q, want %q", target.RemotePath, "/opt/nself")
	}
	if target.EnvName != "staging" {
		t.Errorf("EnvName: got %q, want %q", target.EnvName, "staging")
	}
}

// TestResolveDBRemoteTarget_ServerFlagOverridesPrimary verifies that
// --server picks a named non-primary server instead of the environment's
// primary server.
func TestResolveDBRemoteTarget_ServerFlagOverridesPrimary(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	inv := &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"local": {Name: "local", Kind: "local", Servers: []controlplane.Server{{Name: "local-app", Role: controlplane.RoleApp}}},
			"staging": {
				Name: "staging",
				Kind: "remote",
				Servers: []controlplane.Server{
					{Name: "staging-app", Role: controlplane.RoleApp, Host: "deploy@app.example.com", Primary: true},
					{Name: "staging-db", Role: controlplane.RoleDB, Host: "deploy@db.example.com"},
				},
			},
		},
	}
	if err := controlplane.Write(dir, inv); err != nil {
		t.Fatalf("write inventory: %v", err)
	}

	cmd := newDBRemoteTestCmd()
	if err := cmd.Flags().Set("env", "staging"); err != nil {
		t.Fatalf("set --env: %v", err)
	}
	if err := cmd.Flags().Set("server", "staging-db"); err != nil {
		t.Fatalf("set --server: %v", err)
	}
	target, err := resolveDBRemoteTarget(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.SSHTarget != "deploy@db.example.com" {
		t.Errorf("expected --server to override primary selection; got SSHTarget=%q", target.SSHTarget)
	}
}

// TestFindDBTargetServer_PrimaryPreferred verifies the Primary server is
// picked when no explicit server name is requested.
func TestFindDBTargetServer_PrimaryPreferred(t *testing.T) {
	env := controlplane.Environment{
		Servers: []controlplane.Server{
			{Name: "a", Primary: false},
			{Name: "b", Primary: true},
			{Name: "c", Primary: false},
		},
	}
	srv, ok := findDBTargetServer(env, "")
	if !ok || srv.Name != "b" {
		t.Errorf("expected primary server 'b', got %+v (ok=%v)", srv, ok)
	}
}

// TestFindDBTargetServer_FallsBackToFirst verifies the first server is used
// when no server is marked Primary.
func TestFindDBTargetServer_FallsBackToFirst(t *testing.T) {
	env := controlplane.Environment{
		Servers: []controlplane.Server{
			{Name: "a"},
			{Name: "b"},
		},
	}
	srv, ok := findDBTargetServer(env, "")
	if !ok || srv.Name != "a" {
		t.Errorf("expected fallback to first server 'a', got %+v (ok=%v)", srv, ok)
	}
}

// TestFindDBTargetServer_NotFound verifies a named-but-missing server
// returns ok=false rather than a zero-value match.
func TestFindDBTargetServer_NotFound(t *testing.T) {
	env := controlplane.Environment{Servers: []controlplane.Server{{Name: "a"}}}
	if _, ok := findDBTargetServer(env, "does-not-exist"); ok {
		t.Error("expected ok=false for a server name not present in the environment")
	}
}

// TestShellQuoteArg_EscapesSingleQuotes verifies embedded single quotes are
// escaped POSIX-style so remote command strings can't break out of the
// quoted argument.
func TestShellQuoteArg_EscapesSingleQuotes(t *testing.T) {
	got := shellQuoteArg("it's a test")
	want := `'it'\''s a test'`
	if got != want {
		t.Errorf("shellQuoteArg: got %q, want %q", got, want)
	}
}

// TestShellQuoteArg_Empty verifies an empty string quotes to an empty pair
// of quotes rather than an unquoted empty argument (which a shell would drop).
func TestShellQuoteArg_Empty(t *testing.T) {
	if got := shellQuoteArg(""); got != "''" {
		t.Errorf("shellQuoteArg(\"\"): got %q, want \"''\"", got)
	}
}

// TestWrapRemoteVersionDriftError_DetectsCommandNotFound verifies gap #16:
// a remote "command not found"/"unknown command" style failure is turned
// into an actionable version-drift message instead of a bare wrapped error.
func TestWrapRemoteVersionDriftError_DetectsCommandNotFound(t *testing.T) {
	rt := dbRemoteTarget{SSHTarget: "deploy@staging.example.com", EnvName: "staging"}
	err := wrapRemoteVersionDriftError(rt, []string{"db", "migrate", "up"}, os.ErrDeadlineExceeded, "bash: nself: command not found")
	if err == nil {
		t.Fatal("expected a non-nil error")
	}
	if !strings.Contains(err.Error(), "older version") {
		t.Errorf("expected version-drift guidance in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--version") {
		t.Errorf("expected a suggestion to check --version, got: %v", err)
	}
}

// TestWrapRemoteVersionDriftError_PassesThroughOtherErrors verifies that an
// unrelated remote failure (e.g. a real migration SQL error) is still
// wrapped with context but is NOT misreported as a version-drift issue.
func TestWrapRemoteVersionDriftError_PassesThroughOtherErrors(t *testing.T) {
	rt := dbRemoteTarget{SSHTarget: "deploy@staging.example.com", EnvName: "staging"}
	err := wrapRemoteVersionDriftError(rt, []string{"db", "migrate", "up"}, os.ErrDeadlineExceeded, "ERROR: relation \"foo\" does not exist")
	if err == nil {
		t.Fatal("expected a non-nil error")
	}
	if strings.Contains(err.Error(), "older version") {
		t.Errorf("did not expect version-drift guidance for an unrelated SQL error, got: %v", err)
	}
}

// TestDispatchRemoteIfNeeded_NoFlagsRegistered verifies commands without
// --env/--server registered (e.g. minimal test cobra.Commands elsewhere in
// this package) are treated as local, never erroring on a missing flag.
func TestDispatchRemoteIfNeeded_NoFlagsRegistered(t *testing.T) {
	cmd := &cobra.Command{Use: "status"}
	cmd.SetContext(context.Background())
	handled, err := dispatchRemoteIfNeeded(cmd, "db", "migrate", "status")
	if handled {
		t.Error("expected handled=false when --env/--server are not registered on cmd")
	}
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

// TestDispatchRemoteIfNeeded_LocalIsNotHandled verifies that the default
// (no --env/--server) path returns handled=false so callers proceed with
// their original local implementation unchanged.
func TestDispatchRemoteIfNeeded_LocalIsNotHandled(t *testing.T) {
	cmd := newDBRemoteTestCmd()
	handled, err := dispatchRemoteIfNeeded(cmd, "db", "migrate", "up")
	if handled {
		t.Error("expected handled=false for the default local target")
	}
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

// TestMigrateUpCmd_RemoteFlagsRegistered verifies the real dbMigrateUpCmd,
// dbMigrateStatusCmd, and dbHasuraMetadataApplyCmd (wired in db.go's init())
// carry the new --env/--server flags, not just a throwaway test command.
func TestMigrateUpCmd_RemoteFlagsRegistered(t *testing.T) {
	for _, cmd := range []*cobra.Command{dbMigrateUpCmd, dbMigrateStatusCmd, dbHasuraMetadataApplyCmd} {
		if cmd.Flags().Lookup("env") == nil {
			t.Errorf("%s: expected --env flag to be registered", cmd.Use)
		}
		if cmd.Flags().Lookup("server") == nil {
			t.Errorf("%s: expected --server flag to be registered", cmd.Use)
		}
	}
}

// TestResolveDBRemoteTarget_ProjectRootIsRespected verifies resolution reads
// control-plane inventory relative to the resolved project root (via
// projectRoot()), not just the raw cwd — regression guard for running these
// commands from a subdirectory.
func TestResolveDBRemoteTarget_ProjectRootIsRespected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=test\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	inv := &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"local": {Name: "local", Kind: "local", Servers: []controlplane.Server{{Name: "local-app", Role: controlplane.RoleApp}}},
			"staging": {
				Name:    "staging",
				Kind:    "remote",
				Servers: []controlplane.Server{{Name: "staging-app", Host: "deploy@example.com", Primary: true}},
			},
		},
	}
	if err := controlplane.Write(dir, inv); err != nil {
		t.Fatalf("write inventory: %v", err)
	}
	t.Chdir(dir)

	cmd := newDBRemoteTestCmd()
	if err := cmd.Flags().Set("env", "staging"); err != nil {
		t.Fatalf("set --env: %v", err)
	}
	target, err := resolveDBRemoteTarget(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.SSHTarget != "deploy@example.com" {
		t.Errorf("expected resolution from project root inventory, got SSHTarget=%q", target.SSHTarget)
	}
}

// ── checkRemoteVersionDrift / runRemoteNselfCommand pre-flight (gap #16) ──

// withStubbedSSH replaces runSSHCaptured with fn for the duration of the
// test, restoring the original on cleanup. Callers that need to assert call
// counts/args should close over their own state inside fn.
func withStubbedSSH(t *testing.T, fn func(ctx context.Context, sshArgs []string) (string, error)) {
	t.Helper()
	orig := runSSHCaptured
	runSSHCaptured = fn
	t.Cleanup(func() { runSSHCaptured = orig })
}

// withLocalVersion overrides version.Version for the duration of the test,
// restoring the original on cleanup (mirrors internal/version/version_test.go's
// TestVersion_Overridable pattern).
func withLocalVersion(t *testing.T, v string) {
	t.Helper()
	orig := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = orig })
}

// TestRunRemoteNselfCommand_VersionDriftBlocksRealCommand verifies that a
// mismatched remote nself version (1.0.9) against local (1.2.0) produces an
// error naming both versions and the host, and that the real subcommand is
// never executed after drift is detected (call count stays at 1 — the
// version probe — not 2).
func TestRunRemoteNselfCommand_VersionDriftBlocksRealCommand(t *testing.T) {
	withLocalVersion(t, "1.2.0")

	calls := 0
	withStubbedSSH(t, func(ctx context.Context, sshArgs []string) (string, error) {
		calls++
		return "nself version 1.0.9", nil
	})

	rt := dbRemoteTarget{SSHTarget: "deploy@staging.example.com", EnvName: "staging", RemotePath: "/opt/nself"}
	err := runRemoteNselfCommand(context.Background(), rt, "db", "migrate", "status")
	if err == nil {
		t.Fatal("expected a version-drift error")
	}
	if !strings.Contains(err.Error(), "1.2.0") || !strings.Contains(err.Error(), "1.0.9") {
		t.Errorf("expected error to contain both versions, got: %v", err)
	}
	if !strings.Contains(err.Error(), "deploy@staging.example.com") {
		t.Errorf("expected error to name the host, got: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 SSH call (the probe only, real command must not run), got %d", calls)
	}
}

// TestRunRemoteNselfCommand_MatchingVersionRunsRealCommand verifies that a
// matching remote/local version lets the probe pass and the real subcommand
// execute, with the expected remote cd+nself invocation string.
func TestRunRemoteNselfCommand_MatchingVersionRunsRealCommand(t *testing.T) {
	withLocalVersion(t, "1.2.0")

	calls := 0
	var secondCallArgs []string
	withStubbedSSH(t, func(ctx context.Context, sshArgs []string) (string, error) {
		calls++
		if calls == 1 {
			return "nself version 1.2.0", nil
		}
		secondCallArgs = sshArgs
		return "up to date", nil
	})

	rt := dbRemoteTarget{SSHTarget: "deploy@staging.example.com", EnvName: "staging", RemotePath: "/opt/nself"}
	err := runRemoteNselfCommand(context.Background(), rt, "db", "migrate", "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 SSH calls (probe + real command), got %d", calls)
	}
	joined := strings.Join(secondCallArgs, " ")
	if !strings.Contains(joined, "cd '/opt/nself' && nself 'db' 'migrate' 'status'") {
		t.Errorf("expected second call's remote command to cd into RemotePath and run the subcommand, got: %q", joined)
	}
}

// TestRunRemoteNselfCommand_ANSINoiseParsedCorrectly verifies that an
// old bash-era remote's ANSI-escaped banner ("nself v0.9.9" mixed with
// escape codes) is still correctly parsed as 0.9.9 and compared against a
// different local version, producing a drift error.
func TestRunRemoteNselfCommand_ANSINoiseParsedCorrectly(t *testing.T) {
	withLocalVersion(t, "1.2.0")

	withStubbedSSH(t, func(ctx context.Context, sshArgs []string) (string, error) {
		return "\x1b[32mnself v0.9.9\x1b[0m (build unknown)", nil
	})

	rt := dbRemoteTarget{SSHTarget: "deploy@old.example.com", EnvName: "staging", RemotePath: "/opt/nself"}
	err := runRemoteNselfCommand(context.Background(), rt, "db", "migrate", "status")
	if err == nil {
		t.Fatal("expected a version-drift error")
	}
	if !strings.Contains(err.Error(), "0.9.9") {
		t.Errorf("expected the ANSI-noisy remote version 0.9.9 to be parsed out, got: %v", err)
	}
	if !strings.Contains(err.Error(), "1.2.0") {
		t.Errorf("expected local version 1.2.0 in error, got: %v", err)
	}
}

// TestRunRemoteNselfCommand_ProbeFailureNamesHost verifies that a failed
// probe SSH call (e.g. "command not found"-style output, or a connection
// error) surfaces a clear error naming the host rather than a raw/opaque
// exec error.
func TestRunRemoteNselfCommand_ProbeFailureNamesHost(t *testing.T) {
	withLocalVersion(t, "1.2.0")

	withStubbedSSH(t, func(ctx context.Context, sshArgs []string) (string, error) {
		return "bash: nself: command not found", os.ErrDeadlineExceeded
	})

	rt := dbRemoteTarget{SSHTarget: "deploy@broken.example.com", EnvName: "staging", RemotePath: "/opt/nself"}
	err := runRemoteNselfCommand(context.Background(), rt, "db", "migrate", "status")
	if err == nil {
		t.Fatal("expected an error when the version probe itself fails")
	}
	if !strings.Contains(err.Error(), "deploy@broken.example.com") {
		t.Errorf("expected error to clearly name the host, got: %v", err)
	}
	if !strings.Contains(err.Error(), "staging") {
		t.Errorf("expected error to name the env, got: %v", err)
	}
}

// TestRunRemoteNselfCommand_AllowVersionDriftSkipsProbe verifies that
// AllowVersionDrift=true skips the version probe entirely (no SSH call for
// it) and goes straight to the real remote command.
func TestRunRemoteNselfCommand_AllowVersionDriftSkipsProbe(t *testing.T) {
	withLocalVersion(t, "1.2.0")

	calls := 0
	withStubbedSSH(t, func(ctx context.Context, sshArgs []string) (string, error) {
		calls++
		return "ok", nil
	})

	rt := dbRemoteTarget{SSHTarget: "deploy@staging.example.com", EnvName: "staging", RemotePath: "/opt/nself", AllowVersionDrift: true}
	err := runRemoteNselfCommand(context.Background(), rt, "db", "migrate", "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 SSH call (the real command, no probe) when AllowVersionDrift=true, got %d", calls)
	}
}

// TestRunRemoteNselfCommand_DevLocalVersionSkipsProbe verifies that an
// unparseable local version (e.g. "dev" builds) skips the version check
// silently — no probe call — and runs the command directly.
func TestRunRemoteNselfCommand_DevLocalVersionSkipsProbe(t *testing.T) {
	withLocalVersion(t, "dev")

	calls := 0
	withStubbedSSH(t, func(ctx context.Context, sshArgs []string) (string, error) {
		calls++
		return "ok", nil
	})

	rt := dbRemoteTarget{SSHTarget: "deploy@staging.example.com", EnvName: "staging", RemotePath: "/opt/nself"}
	err := runRemoteNselfCommand(context.Background(), rt, "db", "migrate", "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 SSH call (the real command, no probe) for unparseable local version, got %d", calls)
	}
}

// TestDispatchRemoteIfNeeded_AllowVersionDriftFlagPropagates verifies that
// --allow-version-drift set on cmd propagates into the resolved
// dbRemoteTarget.AllowVersionDrift, so runRemoteNselfCommand actually skips
// the probe end-to-end through dispatchRemoteIfNeeded.
func TestDispatchRemoteIfNeeded_AllowVersionDriftFlagPropagates(t *testing.T) {
	withLocalVersion(t, "1.2.0")

	dir := t.TempDir()
	t.Chdir(dir)

	inv := &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"local": {Name: "local", Kind: "local"},
			"staging": {
				Name:    "staging",
				Kind:    "remote",
				Servers: []controlplane.Server{{Name: "staging-app", Host: "deploy@staging.example.com", RemotePath: "/opt/nself", Primary: true}},
			},
		},
	}
	if err := controlplane.Write(dir, inv); err != nil {
		t.Fatalf("write inventory: %v", err)
	}

	calls := 0
	withStubbedSSH(t, func(ctx context.Context, sshArgs []string) (string, error) {
		calls++
		return "ok", nil
	})

	cmd := newDBRemoteTestCmd()
	if err := cmd.Flags().Set("env", "staging"); err != nil {
		t.Fatalf("set --env: %v", err)
	}
	if err := cmd.Flags().Set("allow-version-drift", "true"); err != nil {
		t.Fatalf("set --allow-version-drift: %v", err)
	}

	handled, err := dispatchRemoteIfNeeded(cmd, "db", "migrate", "status")
	if !handled {
		t.Fatal("expected handled=true for a remote target")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 SSH call (no probe) with --allow-version-drift, got %d", calls)
	}
}

// writeRemoteInventory chdirs into a temp project with one remote "staging"
// environment (deploy@staging.example.com, /opt/nself).
func writeRemoteInventory(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	inv := &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"local": {Name: "local", Kind: "local"},
			"staging": {
				Name: "staging", Kind: "remote",
				Servers: []controlplane.Server{{Name: "staging-app", Host: "deploy@staging.example.com", RemotePath: "/opt/nself", Primary: true}},
			},
		},
	}
	if err := controlplane.Write(dir, inv); err != nil {
		t.Fatalf("write inventory: %v", err)
	}
}

// fakeRemote is what the stubbed remote answers: `nself --version` (the drift
// probe for ordinary commands) with Version, and `nself version --json` (the
// dry-run capability probe) with VersionJSON or ProbeErr.
type fakeRemote struct {
	Version     string
	VersionJSON string
	ProbeErr    error
}

const (
	probeVersion = "nself --version"
	probeCaps    = "nself version --json"
)

// capsJSON is a remote's `nself version --json` output advertising caps.
func capsJSON(version string, caps ...string) string {
	b, _ := json.Marshal(map[string]any{"version": version, "capabilities": caps})
	return string(b)
}

// remoteUpWith runs runDBMigrateUp as `db migrate up --env staging <flags>`
// with local version localVer against the fake remote r. It returns every SSH
// command line, probes included.
func remoteUpWith(t *testing.T, localVer string, r fakeRemote, flags map[string]string) ([]string, error) {
	t.Helper()
	withLocalVersion(t, localVer)
	writeRemoteInventory(t)
	var sent []string
	withStubbedSSH(t, func(ctx context.Context, sshArgs []string) (string, error) {
		line := sshArgs[len(sshArgs)-1]
		sent = append(sent, line)
		switch line {
		case probeVersion:
			return "nself " + r.Version, nil
		case probeCaps:
			return r.VersionJSON, r.ProbeErr
		}
		return "ok", nil
	})
	cmd := newDBRemoteTestCmd()
	cmd.Flags().String("migration-dir", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("plugin", "", "")
	if err := cmd.Flags().Set("env", "staging"); err != nil {
		t.Fatal(err)
	}
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s: %v", k, err)
		}
	}
	return sent, runDBMigrateUp(cmd, nil)
}

// remoteUp is remoteUpWith for a dev local build against a remote reporting
// no capabilities: ordinary (non-dry-run) commands send only the real command.
func remoteUp(t *testing.T, flags map[string]string) ([]string, error) {
	t.Helper()
	return remoteUpWith(t, "dev", fakeRemote{}, flags)
}

// capableRemote advertises db-dry-run-safe, whatever its version number says.
func capableRemote() fakeRemote {
	return fakeRemote{Version: "1.4.12", VersionJSON: capsJSON("1.4.12", version.CapDBDryRunSafe)}
}

// wantRemote asserts the SSH log is exactly [probe, command].
func wantRemote(t *testing.T, sent []string, probe, command string) {
	t.Helper()
	if len(sent) != 2 || sent[0] != probe || sent[1] != command {
		t.Fatalf("ssh commands = %q, want exactly %q", sent, []string{probe, command})
	}
}

// P7-PROD-77: the exact remote command for a directory dry-run. Dropping
// --dry-run or --migration-dir here made a remote "preview" apply the remote's
// default directory for real.
func TestRemoteUp_MigrationDirDryRun_ArgvIsExact(t *testing.T) {
	sent, err := remoteUpWith(t, "1.4.12", capableRemote(), map[string]string{"migration-dir": "migrations", "dry-run": "true"})
	if err != nil {
		t.Fatal(err)
	}
	wantRemote(t, sent, probeCaps, "cd '/opt/nself' && nself 'db' 'migrate' 'up' '--migration-dir' 'migrations' '--dry-run'")
}

func TestRemoteUp_MigrationDirWithoutDryRun_ArgvHasNoDryRun(t *testing.T) {
	sent, err := remoteUp(t, map[string]string{"migration-dir": "migrations"})
	if err != nil {
		t.Fatal(err)
	}
	want := "cd '/opt/nself' && nself 'db' 'migrate' 'up' '--migration-dir' 'migrations'"
	if len(sent) != 1 || sent[0] != want {
		t.Fatalf("remote command = %q, want exactly [%q]", sent, want)
	}
}

// A hostile directory name stays one shell word.
func TestRemoteUp_MigrationDirIsShellQuoted(t *testing.T) {
	sent, err := remoteUpWith(t, "1.4.12", capableRemote(), map[string]string{"migration-dir": "m; rm -rf /", "dry-run": "true"})
	if err != nil {
		t.Fatal(err)
	}
	wantRemote(t, sent, probeCaps, "cd '/opt/nself' && nself 'db' 'migrate' 'up' '--migration-dir' 'm; rm -rf /' '--dry-run'")
}

// `db migrate down` has no --env/--server: it can never be pointed at a remote
// host, so it never sends an SSH command.
func TestDBMigrateDown_NeverDispatchesRemote(t *testing.T) {
	for _, f := range []string{"env", "server", "allow-version-drift"} {
		if dbMigrateDownCmd.Flags().Lookup(f) != nil {
			t.Errorf("db migrate down must not register --%s", f)
		}
	}
	dir := dirProject(t)
	useFakeDocker(t, appliedAnswers(t, dir, "002_b.sql"))
	calls := 0
	withStubbedSSH(t, func(ctx context.Context, sshArgs []string) (string, error) {
		calls++
		return "", nil
	})
	cmd, _ := newDirTestCmd("down")
	_ = cmd.Flags().Set("migration-dir", dir)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Errorf("down sent %d SSH command(s), want 0", calls)
	}
}
