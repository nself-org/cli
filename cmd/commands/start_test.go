package commands

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

// startCmdWithTimeout creates a root+start command tree with a context that
// times out after d, so docker compose operations fail quickly in tests.
func startCmdWithTimeout(t *testing.T, d time.Duration) *cobra.Command {
	t.Helper()
	root := newStartCmd()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	root.SetContext(ctx)
	return root
}

// makeStartFlags creates a cobra.Command with all start flags registered and
// parses the provided flag strings. Used to call resolveStartOpts directly.
func makeStartFlags(t *testing.T, flags ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "start"}
	cmd.Flags().BoolP("verbose", "v", false, "")
	cmd.Flags().BoolP("debug", "d", false, "")
	cmd.Flags().Bool("skip-health-checks", false, "")
	cmd.Flags().Int("timeout", 120, "")
	cmd.Flags().Bool("fresh", false, "")
	cmd.Flags().Bool("force-recreate", false, "")
	cmd.Flags().Bool("clean-start", false, "")
	cmd.Flags().Bool("quick", false, "")
	cmd.Flags().Bool("skip-port-check", false, "")
	cmd.Flags().Bool("skip-build", false, "")
	cmd.Flags().Bool("no-monorepo", false, "")
	cmd.Flags().Bool("skip-db-init", false, "")
	if err := cmd.ParseFlags(flags); err != nil {
		t.Fatalf("makeStartFlags: parsing %v: %v", flags, err)
	}
	return cmd
}

// TestResolveStartOpts_QuickFlagSetsTimeout verifies that --quick overrides
// the timeout to 30 seconds, regardless of the --timeout flag value.
func TestResolveStartOpts_QuickFlagSetsTimeout(t *testing.T) {
	cmd := makeStartFlags(t, "--quick", "--timeout", "120")
	opts, err := resolveStartOpts(cmd)
	if err != nil {
		t.Fatalf("resolveStartOpts: %v", err)
	}
	if opts.timeout != 30 {
		t.Errorf("--quick should set timeout to 30, got %d", opts.timeout)
	}
	if !opts.quick {
		t.Error("--quick should set quick=true")
	}
}

// TestResolveStartOpts_TimeoutClampLow verifies that a timeout below 30 is
// clamped to 30.
func TestResolveStartOpts_TimeoutClampLow(t *testing.T) {
	cmd := makeStartFlags(t, "--timeout", "5")
	opts, err := resolveStartOpts(cmd)
	if err != nil {
		t.Fatalf("resolveStartOpts: %v", err)
	}
	if opts.timeout != 30 {
		t.Errorf("timeout below 30 should be clamped to 30, got %d", opts.timeout)
	}
}

// TestResolveStartOpts_TimeoutClampHigh verifies that a timeout above 600 is
// clamped to 600.
func TestResolveStartOpts_TimeoutClampHigh(t *testing.T) {
	cmd := makeStartFlags(t, "--timeout", "900")
	opts, err := resolveStartOpts(cmd)
	if err != nil {
		t.Fatalf("resolveStartOpts: %v", err)
	}
	if opts.timeout != 600 {
		t.Errorf("timeout above 600 should be clamped to 600, got %d", opts.timeout)
	}
}

// TestResolveStartOpts_ForceRecreateAlias verifies that --force-recreate sets
// fresh=true (it is an alias for --fresh).
func TestResolveStartOpts_ForceRecreateAlias(t *testing.T) {
	cmd := makeStartFlags(t, "--force-recreate")
	opts, err := resolveStartOpts(cmd)
	if err != nil {
		t.Fatalf("resolveStartOpts: %v", err)
	}
	if !opts.fresh {
		t.Error("--force-recreate should set fresh=true via alias")
	}
}

// TestResolveStartOpts_DefaultTimeout verifies that the default timeout (120)
// is within the valid range and is not clamped.
func TestResolveStartOpts_DefaultTimeout(t *testing.T) {
	cmd := makeStartFlags(t)
	opts, err := resolveStartOpts(cmd)
	if err != nil {
		t.Fatalf("resolveStartOpts: %v", err)
	}
	if opts.timeout != 120 {
		t.Errorf("default timeout should be 120, got %d", opts.timeout)
	}
}

// newStartCmd returns a fresh cobra.Command tree wired with just the start
// command so tests can execute it in isolation without side effects from the
// global RootCmd state.
func newStartCmd() *cobra.Command {
	root := &cobra.Command{Use: "nself", RunE: func(cmd *cobra.Command, args []string) error { return nil }}

	bc := &cobra.Command{
		Use:   "start",
		Short: "Boot your nSelf stack",
		RunE:  runStart,
	}
	bc.Flags().BoolP("verbose", "v", false, "Show detailed Docker output")
	bc.Flags().BoolP("debug", "d", false, "Enable debug mode")
	bc.Flags().Bool("skip-health-checks", false, "Skip health validation")
	bc.Flags().Int("timeout", 120, "Health check timeout in seconds")
	bc.Flags().Bool("fresh", false, "Force recreate all containers")
	bc.Flags().Bool("force-recreate", false, "Alias for --fresh")
	bc.Flags().Bool("clean-start", false, "Remove all containers before starting")
	bc.Flags().Bool("quick", false, "Quick start")
	bc.Flags().Bool("skip-port-check", false, "Skip port availability check")
	bc.Flags().Bool("skip-build", false, "Skip automatic rebuild detection")
	bc.Flags().Bool("no-monorepo", false, "Disable monorepo detection")
	bc.Flags().Bool("skip-db-init", false, "Skip database migrations and seed (CI/E2E mode)")

	root.AddCommand(bc)
	return root
}

// TestStartCmd_Registered verifies that the start subcommand is registered on
// the root command.
func TestStartCmd_Registered(t *testing.T) {
	found := false
	for _, c := range RootCmd.Commands() {
		if c.Name() == "start" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'start' to be registered on RootCmd")
	}
}

// TestStartCmd_FlagVerbose verifies that the --verbose flag is accepted by the
// start command without a parse error.
func TestStartCmd_FlagVerbose(t *testing.T) {
	root := newStartCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	// The command will fail because Docker is not running or no nself project
	// exists, but the flag must be recognised (not an "unknown flag" error).
	root.SetArgs([]string{"start", "--verbose", "--no-monorepo"})
	err := root.Execute()

	if err != nil && strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--verbose flag was not recognised: %v", err)
	}
}

// TestStartCmd_FlagSkipHealthChecks verifies that the --skip-health-checks
// flag is accepted.
func TestStartCmd_FlagSkipHealthChecks(t *testing.T) {
	root := newStartCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"start", "--skip-health-checks", "--no-monorepo"})
	err := root.Execute()

	if err != nil && strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--skip-health-checks flag was not recognised: %v", err)
	}
}

// TestStartCmd_FlagTimeout verifies that the --timeout flag is accepted.
func TestStartCmd_FlagTimeout(t *testing.T) {
	root := newStartCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"start", "--timeout", "60", "--no-monorepo"})
	err := root.Execute()

	if err != nil && strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--timeout flag was not recognised: %v", err)
	}
}

// TestStartCmd_UnknownFlagRejected verifies that an unrecognised flag is
// rejected with a cobra error rather than silently ignored.
func TestStartCmd_UnknownFlagRejected(t *testing.T) {
	root := newStartCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"start", "--this-flag-does-not-exist"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error for unknown flag, got nil")
	}
}

// TestStartCmd_NoProjectDir verifies that running start outside an nself
// project returns a meaningful error rather than panicking.
func TestStartCmd_NoProjectDir(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newStartCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"start", "--no-monorepo", "--skip-build"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when running start outside an nself project, got nil")
	}
}

// TestStartCmd_WithProjectDirNoDocker verifies that when a valid nself project
// directory is present (has a .env file) but Docker is not running, start
// returns a Docker-related error rather than "no nself project" error.
// This tests the code path after FindNSelfRoot succeeds.
func TestStartCmd_WithProjectDirNoDocker(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal .env so FindNSelfRoot returns this directory.
	if err := os.WriteFile(dir+"/.env", []byte("PROJECT_NAME=test\nBASE_DOMAIN=test.dev\n"), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	t.Chdir(dir)
	// A stub docker first on PATH keeps the real docker (and its child
	// processes) out of this test; see stubDockerOnPath.
	dockerLog := stubDockerOnPath(t)

	// Use 3-second timeout: this test intentionally hits checkDockerAvailable,
	// which hangs on runners without Docker. Same guard as other start tests.
	root := startCmdWithTimeout(t, 3*time.Second)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"start", "--no-monorepo", "--skip-build"})
	err := root.Execute()

	// The Docker preflight passes (stub), so start must fail at the compose-file
	// check, which this fixture lacks, and must not have run any compose call.
	if err == nil || !errors.Is(err, errs.ErrComposeNotFound) {
		t.Fatalf("want ErrComposeNotFound after the preflight, got: %v", err)
	}
	if stubDockerSawCompose(t, dockerLog) {
		t.Fatal("stub saw a compose call although the compose file is missing")
	}

	// Must fail (Docker likely not running in test env), but should NOT fail
	// with "no nself project" — it should get past FindNSelfRoot.

	// Should not be a "no nself project" error.
	if strings.Contains(err.Error(), "no nself project") {
		t.Errorf("expected to find project dir, but got: %v", err)
	}
}

// TestStartCmd_WithValidProjectSetup verifies that runStart proceeds past the
// compose-file-found and config-loaded stages when a valid project directory
// exists. The test fails at the docker compose up step (expected in a unit test
// environment), but covers the config loading, validation, license heartbeat,
// cert expiry check, and port skip code paths.
func TestStartCmd_WithValidProjectSetup(t *testing.T) {
	dir := t.TempDir()

	// Minimal but valid .env — ApplyDefaults fills in generated passwords, so
	// only PROJECT_NAME and BASE_DOMAIN are strictly needed here.
	envContent := "PROJECT_NAME=testproject\nBASE_DOMAIN=test.example.com\n"
	if err := os.WriteFile(dir+"/.env", []byte(envContent), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	// Minimal docker-compose.yml so Step 1 (compose file check) passes.
	composeContent := "version: '3.8'\nservices:\n  postgres:\n    image: postgres:16\n"
	if err := os.WriteFile(dir+"/docker-compose.yml", []byte(composeContent), 0600); err != nil {
		t.Fatalf("writing docker-compose.yml: %v", err)
	}

	t.Chdir(dir)
	// A stub docker first on PATH keeps the real docker (and its child
	// processes) out of this test; see stubDockerOnPath.
	dockerLog := stubDockerOnPath(t)

	// 3-second timeout: enough for checkDockerAvailable + config load, but
	// causes docker compose up to fail quickly instead of timing out for 60s.
	root := startCmdWithTimeout(t, 3*time.Second)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	// --skip-build:      avoid NeedsRebuild I/O
	// --skip-port-check: avoid port availability check in test env
	// --no-monorepo:     avoid monorepo detection
	root.SetArgs([]string{"start", "--no-monorepo", "--skip-build", "--skip-port-check"})
	err := root.Execute()

	// Reached the compose step: preflight, compose-file check and config load all ran.
	requireReachedCompose(t, err, dockerLog)

	// The command must fail (no real Docker or compose network), but it must
	// NOT fail on early-exit conditions (no project, no compose file, bad config).

	// Acceptable failure reasons: Docker not running, compose up failed, context deadline, etc.
	// Unacceptable: "no nself project", "loading config".
	for _, badMsg := range []string{
		"no nself project",
		"loading config:",
		"config validation failed",
	} {
		if strings.Contains(err.Error(), badMsg) {
			t.Errorf("start failed for wrong reason %q: %v", badMsg, err)
		}
	}
	t.Logf("start failed as expected at runtime stage: %v", err)
}

// TestStartCmd_VerboseFlag verifies the --verbose flag is propagated into
// runStart and covers the verbose config-info output block.
func TestStartCmd_VerboseFlag(t *testing.T) {
	dir := t.TempDir()
	envContent := "PROJECT_NAME=verboseproject\nBASE_DOMAIN=verbose.example.com\n"
	if err := os.WriteFile(dir+"/.env", []byte(envContent), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	composeContent := "version: '3.8'\nservices:\n  postgres:\n    image: postgres:16\n"
	if err := os.WriteFile(dir+"/docker-compose.yml", []byte(composeContent), 0600); err != nil {
		t.Fatalf("writing docker-compose.yml: %v", err)
	}
	t.Chdir(dir)
	// A stub docker first on PATH keeps the real docker (and its child
	// processes) out of this test; see stubDockerOnPath.
	dockerLog := stubDockerOnPath(t)

	root := startCmdWithTimeout(t, 3*time.Second)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"start", "--no-monorepo", "--skip-build", "--skip-port-check", "--verbose"})
	err := root.Execute()

	// Reached the compose step: preflight, compose-file check and config load all ran.
	requireReachedCompose(t, err, dockerLog)

	if strings.Contains(err.Error(), "loading config:") {
		t.Errorf("unexpected config load failure: %v", err)
	}
}

// TestStartCmd_FreshFlag verifies the --fresh flag triggers the compose-down
// step before compose-up, covering the fresh/force-recreate code path.
func TestStartCmd_FreshFlag(t *testing.T) {
	dir := t.TempDir()
	envContent := "PROJECT_NAME=freshproject\nBASE_DOMAIN=fresh.example.com\n"
	if err := os.WriteFile(dir+"/.env", []byte(envContent), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	composeContent := "version: '3.8'\nservices:\n  postgres:\n    image: postgres:16\n"
	if err := os.WriteFile(dir+"/docker-compose.yml", []byte(composeContent), 0600); err != nil {
		t.Fatalf("writing docker-compose.yml: %v", err)
	}
	t.Chdir(dir)
	// A stub docker first on PATH keeps the real docker (and its child
	// processes) out of this test; see stubDockerOnPath.
	dockerLog := stubDockerOnPath(t)

	root := startCmdWithTimeout(t, 3*time.Second)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"start", "--no-monorepo", "--skip-build", "--skip-port-check", "--fresh"})
	err := root.Execute()

	// Reached the compose step: preflight, compose-file check and config load all ran.
	requireReachedCompose(t, err, dockerLog)

	if strings.Contains(err.Error(), "no nself project") || strings.Contains(err.Error(), "loading config:") {
		t.Errorf("unexpected early failure: %v", err)
	}
}

// TestStartCmd_CleanStartFlag verifies the --clean-start flag triggers the
// compose-down cleanup path before start, covering cleanStart code branch.
func TestStartCmd_CleanStartFlag(t *testing.T) {
	dir := t.TempDir()
	envContent := "PROJECT_NAME=cleanproject\nBASE_DOMAIN=clean.example.com\n"
	if err := os.WriteFile(dir+"/.env", []byte(envContent), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	composeContent := "version: '3.8'\nservices:\n  postgres:\n    image: postgres:16\n"
	if err := os.WriteFile(dir+"/docker-compose.yml", []byte(composeContent), 0600); err != nil {
		t.Fatalf("writing docker-compose.yml: %v", err)
	}
	t.Chdir(dir)
	// A stub docker first on PATH keeps the real docker (and its child
	// processes) out of this test; see stubDockerOnPath.
	dockerLog := stubDockerOnPath(t)

	root := startCmdWithTimeout(t, 3*time.Second)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"start", "--no-monorepo", "--skip-build", "--skip-port-check", "--clean-start"})
	err := root.Execute()

	// Reached the compose step: preflight, compose-file check and config load all ran.
	requireReachedCompose(t, err, dockerLog)

	if strings.Contains(err.Error(), "no nself project") || strings.Contains(err.Error(), "loading config:") {
		t.Errorf("unexpected early failure: %v", err)
	}
}

// TestStartCmd_AlreadyRunning verifies that the --already-running error path
// produces a descriptive error. Uses an env file with no docker-compose.yml
// to exercise the docker-compose.yml-not-found path.
func TestStartCmd_ComposeFileNotFound(t *testing.T) {
	dir := t.TempDir()

	// Create .env so FindNSelfRoot succeeds, but NO docker-compose.yml.
	if err := os.WriteFile(dir+"/.env", []byte("PROJECT_NAME=test\nBASE_DOMAIN=test.dev\n"), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	t.Chdir(dir)
	// A stub docker first on PATH keeps the real docker (and its child
	// processes) out of this test; see stubDockerOnPath.
	dockerLog := stubDockerOnPath(t)

	// Use 3-second timeout to avoid hanging on Docker check (same as other start tests).
	root := startCmdWithTimeout(t, 3*time.Second)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"start", "--no-monorepo", "--skip-build"})
	err := root.Execute()

	// The preflight passes (stub); the missing compose file is the failure.
	if err == nil || !errors.Is(err, errs.ErrComposeNotFound) {
		t.Fatalf("want ErrComposeNotFound after the preflight, got: %v", err)
	}
	if stubDockerSawCompose(t, dockerLog) {
		t.Fatal("stub saw a compose call although the compose file is missing")
	}

	// If Docker is not available the error is Docker-related (expected).
	// If Docker IS available but no compose file exists the error should mention compose.
	t.Logf("start returned error (expected): %v", err)
}

// ── --skip-db-init tests ──────────────────────────────────────────────────────

// TestResolveStartOpts_SkipDBInitFlag verifies that --skip-db-init is parsed
// into startOpts.skipDBInit=true.
func TestResolveStartOpts_SkipDBInitFlag(t *testing.T) {
	cmd := makeStartFlags(t, "--skip-db-init")
	opts, err := resolveStartOpts(cmd)
	if err != nil {
		t.Fatalf("resolveStartOpts: %v", err)
	}
	if !opts.skipDBInit {
		t.Error("--skip-db-init should set skipDBInit=true")
	}
}

// TestResolveStartOpts_SkipDBInitDefaultFalse verifies that skipDBInit
// defaults to false when the flag is not supplied and the env var is unset.
func TestResolveStartOpts_SkipDBInitDefaultFalse(t *testing.T) {
	t.Setenv("NSELF_SKIP_DB_INIT", "") // ensure env var is not set
	cmd := makeStartFlags(t)
	opts, err := resolveStartOpts(cmd)
	if err != nil {
		t.Fatalf("resolveStartOpts: %v", err)
	}
	if opts.skipDBInit {
		t.Error("skipDBInit should default to false when flag and env var are absent")
	}
}

// TestResolveStartOpts_SkipDBInitEnvVar verifies that NSELF_SKIP_DB_INIT=true
// activates skipDBInit even when the CLI flag is not passed.
func TestResolveStartOpts_SkipDBInitEnvVar(t *testing.T) {
	t.Setenv("NSELF_SKIP_DB_INIT", "true")
	cmd := makeStartFlags(t) // no --skip-db-init flag
	opts, err := resolveStartOpts(cmd)
	if err != nil {
		t.Fatalf("resolveStartOpts: %v", err)
	}
	if !opts.skipDBInit {
		t.Error("NSELF_SKIP_DB_INIT=true should activate skipDBInit")
	}
}

// TestStartCmd_FlagSkipDBInit verifies that --skip-db-init is accepted by the
// start command and does not produce an "unknown flag" error.
func TestStartCmd_FlagSkipDBInit(t *testing.T) {
	root := newStartCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"start", "--skip-db-init", "--no-monorepo"})
	err := root.Execute()

	if err != nil && strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--skip-db-init flag was not recognised: %v", err)
	}
}

// TestStartCmd_SkipDBInitSkipsMigrationsAndReachesServiceStart verifies that
// when --skip-db-init is set on a valid project directory, the command proceeds
// past the migration step (no "database init:" error) before failing at runtime
// (compose up / Docker not running). This confirms the migration/seed path is
// bypassed.
func TestStartCmd_SkipDBInitSkipsMigrationsAndReachesServiceStart(t *testing.T) {
	dir := t.TempDir()

	envContent := "PROJECT_NAME=ciproject\nBASE_DOMAIN=ci.example.com\n"
	if err := os.WriteFile(dir+"/.env", []byte(envContent), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	composeContent := "version: '3.8'\nservices:\n  postgres:\n    image: postgres:16\n"
	if err := os.WriteFile(dir+"/docker-compose.yml", []byte(composeContent), 0600); err != nil {
		t.Fatalf("writing docker-compose.yml: %v", err)
	}

	t.Chdir(dir)
	// A stub docker first on PATH keeps the real docker (and its child
	// processes) out of this test; see stubDockerOnPath.
	dockerLog := stubDockerOnPath(t)

	root := startCmdWithTimeout(t, 3*time.Second)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"start", "--no-monorepo", "--skip-build", "--skip-port-check", "--skip-db-init"})
	err := root.Execute()

	// Reached the compose step: preflight, compose-file check and config load all ran.
	requireReachedCompose(t, err, dockerLog)

	// The command must NOT fail with a "database init:" error, which would
	// indicate the migration path was executed despite --skip-db-init.
	if strings.Contains(err.Error(), "database init:") {
		t.Errorf("--skip-db-init should bypass database init, but got: %v", err)
	}

	// Acceptable failures: Docker not running, compose up, context deadline
	// (all runtime failures that prove we got past the migration step).
	t.Logf("start failed at expected runtime stage (migration skipped): %v", err)
}

// TestCIReadyServices verifies that the ciReadyServices constant contains the
// three required CI backend services.
func TestCIReadyServices(t *testing.T) {
	required := map[string]bool{"postgres": false, "hasura": false, "auth": false}
	for _, svc := range ciReadyServices {
		required[svc] = true
	}
	for svc, found := range required {
		if !found {
			t.Errorf("ciReadyServices is missing required CI service %q", svc)
		}
	}
	if len(ciReadyServices) != 3 {
		t.Errorf("ciReadyServices should have exactly 3 entries, got %d: %v", len(ciReadyServices), ciReadyServices)
	}
}
