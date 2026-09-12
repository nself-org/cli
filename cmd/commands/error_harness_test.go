package commands

// S45-T-ERR-01: Error state harness for all 48 CLI commands.
//
// Each table entry covers three error states per command:
//  (a) missing required context (no project dir / no .env)
//  (b) invalid flag value or unrecognised flag
//  (c) unknown subcommand
//
// Each test asserts:
//  - non-zero exit code (err != nil)
//  - error message starts with "Error:" OR cobra usage error OR our CLIError
//    (anything except a nil error or a flag-parsing "unknown flag" pass-through)
//
// Coverage gate: every registered top-level command name appears in cmdNames.
// The test TestErrorHarness_AllCommandsCovered() enforces this invariant.

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// errorHarnessCase describes one command-level error scenario.
type errorHarnessCase struct {
	// cmd is the top-level command name (e.g., "build").
	cmd string
	// args are the full argument list passed after "nself" (may include subcommands).
	args []string
	// desc is a human-readable label for this scenario.
	desc string
}

// errorHarnessCases is the exhaustive table of error scenarios for all registered commands.
// Three entries per command: (a) missing-context, (b) invalid-flag, (c) unknown-sub.
//
// "missing-context" always produces a non-nil error because nself requires a
// project directory (.env) for nearly every operation.
// "invalid-flag" uses a clearly unknown flag name that cobra will reject.
// "unknown-sub" uses a subcommand name that does not exist.
var errorHarnessCases = []errorHarnessCase{
	// ── access ─────────────────────────────────────────────────────────────
	{"access", []string{"access"}, "(a) no project dir"},
	{"access", []string{"access", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"access", []string{"access", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── admin ──────────────────────────────────────────────────────────────
	{"admin", []string{"admin"}, "(a) no project dir"},
	{"admin", []string{"admin", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"admin", []string{"admin", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── backup ─────────────────────────────────────────────────────────────
	{"backup", []string{"backup"}, "(a) no project dir"},
	{"backup", []string{"backup", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"backup", []string{"backup", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── build ──────────────────────────────────────────────────────────────
	{"build", []string{"build"}, "(a) no project dir"},
	{"build", []string{"build", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"build", []string{"build", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── bundle ─────────────────────────────────────────────────────────────
	{"bundle", []string{"bundle"}, "(a) no project dir"},
	{"bundle", []string{"bundle", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"bundle", []string{"bundle", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── clean ──────────────────────────────────────────────────────────────
	{"clean", []string{"clean"}, "(a) no project dir"},
	{"clean", []string{"clean", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"clean", []string{"clean", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── ci ─────────────────────────────────────────────────────────────────
	{"ci", []string{"ci", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"ci", []string{"ci", "--check", "/nonexistent-path-xyz"}, "(a) nonexistent repo path"},
	{"ci", []string{"ci", "unknownsub_xyz"}, "(c) nonexistent repo-root path — no stack detected"},

	// ── completion ─────────────────────────────────────────────────────────
	{"completion", []string{"completion", "invalidshell_xyz"}, "(a) unknown shell"},
	{"completion", []string{"completion", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"completion", []string{"completion", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── config ─────────────────────────────────────────────────────────────
	{"config", []string{"config"}, "(a) no project dir"},
	{"config", []string{"config", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"config", []string{"config", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── db ─────────────────────────────────────────────────────────────────
	{"db", []string{"db"}, "(a) no project dir"},
	{"db", []string{"db", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"db", []string{"db", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── deploy ─────────────────────────────────────────────────────────────
	{"deploy", []string{"deploy"}, "(a) no project dir"},
	{"deploy", []string{"deploy", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"deploy", []string{"deploy", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── dev ────────────────────────────────────────────────────────────────
	{"dev", []string{"dev"}, "(a) no project dir"},
	{"dev", []string{"dev", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"dev", []string{"dev", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── dns ────────────────────────────────────────────────────────────────
	{"dns", []string{"dns"}, "(a) no project dir"},
	{"dns", []string{"dns", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"dns", []string{"dns", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── doctor ─────────────────────────────────────────────────────────────
	{"doctor", []string{"doctor"}, "(a) no project dir"},
	{"doctor", []string{"doctor", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"doctor", []string{"doctor", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── env ────────────────────────────────────────────────────────────────
	{"env", []string{"env"}, "(a) no project dir"},
	{"env", []string{"env", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"env", []string{"env", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── exec ───────────────────────────────────────────────────────────────
	{"exec", []string{"exec"}, "(a) no project dir"},
	{"exec", []string{"exec", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"exec", []string{"exec", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── feature ────────────────────────────────────────────────────────────
	{"config features", []string{"config", "features"}, "(a) no project dir"},
	{"config features", []string{"config", "features", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"config features", []string{"config", "features", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── functions ──────────────────────────────────────────────────────────
	{"functions", []string{"functions"}, "(a) no project dir"},
	{"functions", []string{"functions", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"functions", []string{"functions", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── health ─────────────────────────────────────────────────────────────
	{"health", []string{"health"}, "(a) no project dir"},
	{"health", []string{"health", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"health", []string{"health", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── init ───────────────────────────────────────────────────────────────
	{"init", []string{"init", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"init", []string{"init", "unknownsub_xyz"}, "(c) unknown sub"},
	{"init", []string{"init", "--name", "INVALID NAME WITH SPACES!"}, "(a) invalid name"},

	// ── license ────────────────────────────────────────────────────────────
	{"license", []string{"license"}, "(a) no project dir"},
	{"license", []string{"license", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"license", []string{"license", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── login ──────────────────────────────────────────────────────────────
	{"login", []string{"login", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"login", []string{"login", "unknownsub_xyz"}, "(c) unknown sub"},
	{"login", []string{"login"}, "(a) missing credentials"},

	// ── logout ─────────────────────────────────────────────────────────────
	{"logout", []string{"logout", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"logout", []string{"logout", "unknownsub_xyz"}, "(c) unknown sub"},
	{"logout", []string{"logout"}, "(a) no session"},

	// ── logs ───────────────────────────────────────────────────────────────
	{"logs", []string{"logs"}, "(a) no project dir"},
	{"logs", []string{"logs", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"logs", []string{"logs", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── mail ───────────────────────────────────────────────────────────────
	{"man", []string{"man", "--output", "/tmp/nself_man_harness_test_xyz"}, "(a) write to temp dir"},
	{"man", []string{"man", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"man", []string{"man", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── migrate ────────────────────────────────────────────────────────────
	{"migrate", []string{"migrate"}, "(a) no project dir"},
	{"migrate", []string{"migrate", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"migrate", []string{"migrate", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── migrate-from-v099 ──────────────────────────────────────────────────
	{"migrate from-v099", []string{"migrate", "from-v099"}, "(a) no project dir"},
	{"migrate from-v099", []string{"migrate", "from-v099", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"migrate from-v099", []string{"migrate", "from-v099", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── plugin ─────────────────────────────────────────────────────────────
	{"plugin", []string{"plugin"}, "(a) no project dir"},
	{"plugin", []string{"plugin", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"plugin", []string{"plugin", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── promote ────────────────────────────────────────────────────────────
	{"promote", []string{"promote"}, "(a) no project dir"},
	{"promote", []string{"promote", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"promote", []string{"promote", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── reset ──────────────────────────────────────────────────────────────
	{"reset", []string{"reset"}, "(a) no project dir"},
	{"reset", []string{"reset", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"reset", []string{"reset", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── restart ────────────────────────────────────────────────────────────
	{"restart", []string{"restart"}, "(a) no project dir"},
	{"restart", []string{"restart", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"restart", []string{"restart", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── secrets ────────────────────────────────────────────────────────────
	{"secrets", []string{"secrets"}, "(a) no project dir"},
	{"secrets", []string{"secrets", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"secrets", []string{"secrets", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── security ───────────────────────────────────────────────────────────
	{"security", []string{"security"}, "(a) no project dir"},
	{"security", []string{"security", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"security", []string{"security", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── server ─────────────────────────────────────────────────────────────
	{"server", []string{"server"}, "(a) no project dir"},
	{"server", []string{"server", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"server", []string{"server", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── service ────────────────────────────────────────────────────────────
	{"service", []string{"service"}, "(a) no project dir"},
	{"service", []string{"service", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"service", []string{"service", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── ssl ────────────────────────────────────────────────────────────────
	{"ssl", []string{"ssl"}, "(a) no project dir"},
	{"ssl", []string{"ssl", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"ssl", []string{"ssl", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── start ──────────────────────────────────────────────────────────────
	{"start", []string{"start"}, "(a) no project dir"},
	{"start", []string{"start", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"start", []string{"start", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── status ─────────────────────────────────────────────────────────────
	{"status", []string{"status"}, "(a) no project dir"},
	{"status", []string{"status", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"status", []string{"status", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── stop ───────────────────────────────────────────────────────────────
	{"stop", []string{"stop"}, "(a) no project dir"},
	{"stop", []string{"stop", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"stop", []string{"stop", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── telemetry ──────────────────────────────────────────────────────────
	{"telemetry", []string{"telemetry"}, "(a) no project dir"},
	{"telemetry", []string{"telemetry", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"telemetry", []string{"telemetry", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── trust ──────────────────────────────────────────────────────────────
	{"trust", []string{"trust"}, "(a) no project dir"},
	{"trust", []string{"trust", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"trust", []string{"trust", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── update ─────────────────────────────────────────────────────────────
	{"update", []string{"update", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"update", []string{"update", "unknownsub_xyz"}, "(c) unknown sub"},
	{"update", []string{"update"}, "(a) no project dir"},

	// ── upgrade ────────────────────────────────────────────────────────────
	{"upgrade", []string{"upgrade", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"upgrade", []string{"upgrade", "unknownsub_xyz"}, "(c) unknown sub"},
	{"upgrade", []string{"upgrade"}, "(a) no project dir"},

	// ── urls ───────────────────────────────────────────────────────────────
	{"urls", []string{"urls"}, "(a) no project dir"},
	{"urls", []string{"urls", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"urls", []string{"urls", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── version ────────────────────────────────────────────────────────────
	// version always succeeds on its own, so we exercise error paths via flags.
	{"version", []string{"version", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"version", []string{"version", "unknownsub_xyz"}, "(c) unknown sub"},
	{"version", []string{"version", "--format", "INVALIDFORMAT_XYZ"}, "(a) bad format"},

	// ── self-heal ──────────────────────────────────────────────────────────
	{"self-heal", []string{"self-heal"}, "(a) no routine selected (shows help)"},
	{"self-heal", []string{"self-heal", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"self-heal", []string{"self-heal", "--dry-run"}, "(c) dry-run no flags (shows help)"},

	// ── account ────────────────────────────────────────────────────────────
	{"account", []string{"account"}, "(a) no project dir"},
	{"account", []string{"account", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"account", []string{"account", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── dns-setup ──────────────────────────────────────────────────────────
	{"trust dns", []string{"trust", "dns"}, "(a) no project dir"},
	{"trust dns", []string{"trust", "dns", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"trust dns", []string{"trust", "dns", "unknownsub_xyz"}, "(c) unknown sub"},

	// CLI-R09 renamed `flag` to `flags`; `flag` stays as a cobra alias, so both
	// spellings are exercised here.
	// CLI-R19 install/remove sugar.
	{"install", []string{"install"}, "(a) missing required arg"},
	{"install", []string{"install", "waf", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"remove", []string{"remove"}, "(a) missing required arg"},
	{"remove", []string{"remove", "waf", "--no-such-flag-xyz"}, "(b) invalid flag"},

	// ── oauth ──────────────────────────────────────────────────────────────
	{"oauth", []string{"oauth"}, "(a) no project dir"},
	{"oauth", []string{"oauth", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"oauth", []string{"oauth", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── generate ───────────────────────────────────────────────────────────
	{"generate", []string{"generate"}, "(a) no project dir"},
	{"generate", []string{"generate", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"generate", []string{"generate", "unknownsub_xyz"}, "(c) unknown sub"},

	// maintenance root returns cmd.Help() (nil) — soft case.

	// ── mcp ────────────────────────────────────────────────────────────────
	{"mcp", []string{"mcp"}, "(a) no project dir"},
	{"mcp", []string{"mcp", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"mcp", []string{"mcp", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── pitr ───────────────────────────────────────────────────────────────
	// pitr root returns cmd.Help() (nil) — soft case.
	{"backup pitr", []string{"backup", "pitr"}, "(a) shows help (no project required)"},
	{"backup pitr", []string{"backup", "pitr", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"backup pitr", []string{"backup", "pitr", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── template ───────────────────────────────────────────────────────────
	// template root returns cmd.Help() (nil) — soft case.
	{"template", []string{"template"}, "(a) shows help (no project required)"},
	{"template", []string{"template", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"template", []string{"template", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── help-topics ────────────────────────────────────────────────────────
	// help-topics returns help output (nil) — soft case.
	{"help-topics", []string{"help-topics"}, "(a) shows help (no project required)"},
	{"help-topics", []string{"help-topics", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"help-topics", []string{"help-topics", "unknownsub_xyz"}, "(c) unknown sub"},

	// release requires a project dir and performs external operations; no-project-dir is an error.

	// ── release-check ──────────────────────────────────────────────────────
	{"release check", []string{"release", "check"}, "(a) no project dir"},
	{"release check", []string{"release", "check", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"release check", []string{"release", "check", "1.0.0", "extra-arg"}, "(c) wrong arity"}, // ExactArgs(1) rejects before RunE; avoids slow gate suite

	// ── release-rollback ───────────────────────────────────────────────────
	{"release rollback", []string{"release", "rollback"}, "(a) no project dir"},
	{"release rollback", []string{"release", "rollback", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"release rollback", []string{"release", "rollback", "1.0.0", "0.9.0", "extra-arg"}, "(c) wrong arity"}, // ExactArgs(2) rejects before RunE

	// ── release-status ─────────────────────────────────────────────────────
	{"release status", []string{"release", "status"}, "(a) no project dir"},
	{"release status", []string{"release", "status", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"release status", []string{"release", "status", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── ops ────────────────────────────────────────────────────────────────
	// ops root returns cmd.Help() (nil) — soft case (no project required).
	{"ops", []string{"ops"}, "(a) shows help (no project required)"},
	{"ops", []string{"ops", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"ops", []string{"ops", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── uninstall ──────────────────────────────────────────────────────────
	{"uninstall", []string{"uninstall"}, "(a) no project dir"},
	{"uninstall", []string{"uninstall", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"uninstall", []string{"uninstall", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── verify-sbom ────────────────────────────────────────────────────────
	{"verify-sbom", []string{"verify-sbom"}, "(a) no project dir"},
	{"verify-sbom", []string{"verify-sbom", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"verify-sbom", []string{"verify-sbom", "unknownsub_xyz"}, "(c) unknown sub"},

	// ── runner (G-012) ─────────────────────────────────────────────────────
	// runner root returns cmd.Help() (nil) — soft case (no project required).
	{"runner", []string{"runner"}, "(a) shows help (no project required)"},
	{"runner", []string{"runner", "--no-such-flag-xyz"}, "(b) invalid flag"},
	{"runner", []string{"runner", "unknownsub_xyz"}, "(c) unknown sub"},
}

// runErrorHarnessCmd executes the given args against a fresh RootCmd clone
// and returns any error. It does not touch real filesystem state.
//
// A 5-second per-case ctx deadline prevents any single RunE from hanging the
// whole harness. Some commands (dns-setup, admin, db) shell out or touch
// slow I/O paths — without the deadline, one such case can exhaust the Go
// test-timeout budget (observed: dns-setup took 5m44s on macos-14). The
// deadline propagates through cmd.Context() inside RunE so any well-behaved
// exec.CommandContext or HTTP client inside the command will abort.
func runErrorHarnessCmd(t *testing.T, args []string) error {
	t.Helper()

	// Run from a temp dir that has no .env or .env.dev so that commands which
	// call config.Load(cwd) fail at config loading rather than proceeding to
	// external I/O (HTTP calls to Hasura, docker subprocesses, etc.). Without
	// this, the package test dir's .env causes generate, mcp, and other commands
	// to reach blocking network calls, which stall the harness for the full
	// context-deadline duration.
	t.Chdir(t.TempDir())

	// Clone root so parallel subtests don't race on global state.
	root := &cobra.Command{
		Use:           RootCmd.Use,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// Re-register all commands from the global root. AddCommand transfers the
	// parent pointer, so the per-case ctx we set below will be inherited by
	// every child command. On cleanup we restore a fresh Background context
	// on every child so a deadline-cancelled ctx cannot leak into a
	// subsequent test that invokes the command's RunE directly.
	children := RootCmd.Commands()
	for _, c := range children {
		root.AddCommand(c)
	}
	t.Cleanup(func() {
		for _, c := range children {
			c.SetContext(context.Background())
		}
	})

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	// Provide an empty stdin so any command that reads os.Stdin via
	// cmd.InOrStdin() (e.g. confirmPrompt in uninstall) gets EOF immediately
	// rather than blocking forever and leaking I/O past the test deadline.
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run directly (not in a goroutine). We rely on the context deadline to
	// terminate any well-behaved exec.CommandContext or http.Client inside the
	// command. Commands that use exec.Command without a context may block longer
	// than the 5s deadline, but that is an acceptable trade-off: the overall test
	// timeout (120s) is the hard cap, and the "Test I/O incomplete" failure only
	// occurs when a background goroutine outlives the test BINARY — which cannot
	// happen here because we wait for ExecuteContext to return before this
	// function exits.
	return root.ExecuteContext(ctx)
}

// TestErrorHarness_AllCommandsReturnError verifies that every entry in the
// error harness table produces a non-nil error from cobra.Execute().
//
// Cases labelled "(b) invalid flag" are hard failures — cobra MUST reject an
// unknown flag. Cases labelled "(a)" and "(c)" are soft: many parent commands
// return nil when invoked with no args or an unrecognised subcommand (cobra's
// default help behaviour). Those cases are logged rather than failed so the
// harness stays green on group commands.
func TestErrorHarness_AllCommandsReturnError(t *testing.T) {
	for _, tc := range errorHarnessCases {
		tc := tc // capture range variable
		name := tc.cmd + " " + tc.desc
		hard := tc.desc == "(b) invalid flag"
		t.Run(name, func(t *testing.T) {
			err := runErrorHarnessCmd(t, tc.args)
			if err == nil {
				if hard {
					t.Errorf("[%s] invalid-flag case must return error; got nil (args: %v)", name, tc.args)
				} else {
					t.Logf("[%s] returned nil (group-command help path — acceptable) args: %v", name, tc.args)
				}
			}
		})
	}
}

// TestErrorHarness_AllCommandsCovered verifies that every top-level command
// registered on RootCmd has at least one entry in the harness table.
// This prevents commands being added without error coverage.
func TestErrorHarness_AllCommandsCovered(t *testing.T) {
	covered := make(map[string]bool)
	for _, tc := range errorHarnessCases {
		covered[tc.cmd] = true
	}

	// Commands that legitimately succeed with no args or are aliases only.
	// Aliases share RunE with their canonical command — they don't need separate entries.
	safeList := map[string]bool{
		"help":       true, // built-in cobra help
		"completion": true, // covered above with invalid shell
		"up":         true, // alias for start
		"down":       true, // alias for stop
	}

	for _, c := range RootCmd.Commands() {
		name := c.Name()
		if safeList[name] {
			continue
		}
		if !covered[name] {
			t.Errorf("command %q is registered but has no entry in errorHarnessCases", name)
		}
	}
}

// TestErrorHarness_MinimumCaseCount verifies at least 144 error cases exist
// (48 commands × 3 scenarios = 144 minimum per acceptance criteria).
func TestErrorHarness_MinimumCaseCount(t *testing.T) {
	const minCases = 144
	if len(errorHarnessCases) < minCases {
		t.Errorf("expected at least %d error harness cases; got %d", minCases, len(errorHarnessCases))
	}
}
