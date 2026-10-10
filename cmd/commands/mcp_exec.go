package commands

// mcp_exec.go — subprocess helpers for MCP tool handlers.
//
// Purpose: CLI-R15 replaces the old MCP server's `exec.Command("nself", ...)`
//   calls, which shelled out to whatever "nself" happened to resolve to on
//   PATH — potentially a different version, or nothing at all if the server
//   was started via an absolute path outside PATH. Every place this file
//   re-invokes the CLI itself uses os.Executable() instead.
// Inputs:  command name/args (mcpRunExternal) or nself subcommand args
//   (mcpExecSelf); an env var override for tests (see selfExecutablePath).
// Outputs: combined stdout+stderr as a string, and the process error (if
//   any) — callers decide whether a non-zero exit is itself an error to
//   surface (e.g. `nself doctor` legitimately exits non-zero on warnings).
// Constraints: mcpExecSelf is reserved for the handful of commands
//   (build/start/stop/restart/plugin install) whose logic lives in
//   cmd/commands/*.go RunE functions that (a) print operator-facing banners
//   to os.Stdout — which would corrupt the JSON-RPC stream on the stdio
//   transport if called in-process — and, for start/stop, (b) install a
//   process-wide SIGINT/SIGTERM->os.Exit trap (internal/lifecycle.TrapSignals)
//   intended for a short-lived CLI invocation, not a long-lived MCP server
//   process. Re-exec sidesteps both: the trap and any stdout writes happen
//   in the child process, never in the server itself. Every other tool in
//   this package calls the internal data-layer package function directly.
// SPORT: CLI-CMD-MCP-001

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/invoke"
)

// mcpSelfExecOverride, when set, replaces os.Executable() as the binary
// mcpExecSelf re-invokes. It exists solely so integration tests can point
// re-exec at a binary built from this checkout instead of the `go test`
// harness binary (which os.Executable() would otherwise resolve to and
// which doesn't understand "build"/"start"/"stop"/"restart" as arguments).
const mcpSelfExecOverrideEnv = invoke.SelfExecOverrideEnv

// selfExecutablePath resolves the binary mcpExecSelf should invoke. The rule
// (override env, else os.Executable) lives in internal/invoke so the legacy
// handlers and the machine invoker resolve the child the same way.
func selfExecutablePath() (string, error) { return invoke.SelfExecutable() }

// mcpExecTimeout bounds every re-exec'd nself subcommand so a hung child
// process (e.g. waiting on a Docker pull) can't wedge a tool call forever.
const mcpExecTimeout = 5 * time.Minute

// mcpExecSelf re-invokes the nself binary (never the bare "nself" string —
// see selfExecutablePath) with the given subcommand arguments, in dir, and
// returns combined stdout+stderr.
func mcpExecSelf(ctx context.Context, dir string, args ...string) (string, error) {
	bin, err := selfExecutablePath()
	if err != nil {
		return "", fmt.Errorf("resolving nself executable: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, mcpExecTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	// NSELF_NONINTERACTIVE mirrors what CI already sets for these commands —
	// no prompts, since there is no terminal on the other end of an MCP call.
	cmd.Env = append(os.Environ(), "NSELF_NONINTERACTIVE=1", "CI=1")

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	return strings.TrimSpace(out.String()), runErr
}

// mcpRunExternal runs a non-nself external binary (docker, psql) in dir and
// returns combined stdout+stderr. Used for tools whose data source is a
// third-party CLI that this repo has no Go client for.
func mcpRunExternal(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
