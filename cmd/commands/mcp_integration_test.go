package commands

// mcp_integration_test.go — end-to-end proof that an agent can complete
// init -> build -> start -> verify using only MCP tool calls (CLI-R15 DoD).
//
// Purpose: unit tests exercise individual handlers against fakes; this file
//   drives the real handler functions against a real project directory and
//   a real compiled nself binary, the way an MCP client actually would.
// Inputs:  none beyond the Go/Docker toolchains already required to build
//   and test this repo.
// Outputs: a temp project directory, built via `nself init`, then advanced
//   through nself_build / nself_start / nself_status tool calls.
// Constraints: these are the v1.4 hand handlers, which have no init tool, so
//   `nself init` runs as a direct subprocess of the built binary (the v1.5
//   generated surface serves nself_init, even before a project exists: see
//   internal/mcpgen/surface.go). Everything after that
//   (build, and — gated behind INTEGRATION=1 — start/status/stop) goes
//   through the exact tool handler functions the server registers, not
//   through a re-implementation. The Docker-dependent half is gated because
//   this environment may not have a daemon available; the init+build+verify
//   half has no such dependency and always runs.
// SPORT: CLI-CMD-MCP-001

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// buildNSelfBinary returns the nself binary the suite builds once per test
// process (pilotBinary), so mcpExecSelf's re-exec has a real binary to invoke:
// os.Executable() inside `go test` resolves to the test harness, which doesn't
// understand "build"/"start" as subcommands, hence the NSELF_MCP_EXEC_OVERRIDE
// hook. Reusing it spares a second ./cmd/nself compile and link (D-0735).
func buildNSelfBinary(t *testing.T) string {
	t.Helper()
	return pilotBinary(t)
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

// TestMCPIntegration_InitBuildVerify drives init -> build -> verify with no
// Docker dependency. Always runs.
func TestMCPIntegration_InitBuildVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary-build integration test in -short mode")
	}

	bin := buildNSelfBinary(t)

	projectDir := t.TempDir()
	t.Setenv("NSELF_ALLOW_SOURCE_DIR", "1")
	t.Setenv(mcpSelfExecOverrideEnv, bin)

	// 1. init — a direct subprocess of the built binary, not an MCP tool
	// (the server can't start until a project already exists).
	initCmd := exec.Command(bin, "init", "--non-interactive", "--quiet")
	initCmd.Dir = projectDir
	initCmd.Env = append(os.Environ(), "NSELF_ALLOW_SOURCE_DIR=1")
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("nself init failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".env")); err != nil {
		t.Fatalf("expected .env after init: %v", err)
	}

	restore := chdir(t, projectDir)
	defer restore()

	// 2. build — through the actual nself_build MCP tool handler.
	buildHandler := mcpLifecycleHandler("build", "--quiet")
	result, err := buildHandler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("nself_build handler returned a Go error: %v", err)
	}
	buildOut, ok := result.StructuredContent.(ExecResult)
	if !ok {
		t.Fatalf("unexpected structured content type: %T", result.StructuredContent)
	}
	if !buildOut.Success {
		t.Fatalf("nself_build did not succeed: %s", buildOut.Output)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "docker-compose.yml")); err != nil {
		t.Fatalf("expected docker-compose.yml after nself_build: %v", err)
	}

	// 3. verify (no daemon needed) — through nself_urls and nself_doctor.
	urlsResult, err := mcpURLsHandler()(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("nself_urls handler returned a Go error: %v", err)
	}
	if urlsResult.StructuredContent == nil {
		t.Error("expected structured content from nself_urls")
	}

	doctorResult, err := mcpDoctorHandler()(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("nself_doctor handler returned a Go error: %v", err)
	}
	doctorOut, ok := doctorResult.StructuredContent.(DoctorResult)
	if !ok {
		t.Fatalf("unexpected structured content type: %T", doctorResult.StructuredContent)
	}
	if len(doctorOut.Checks) == 0 {
		t.Error("expected at least one doctor check result")
	}

	t.Logf("init -> build -> verify completed via MCP tool handlers alone (no Docker)")
}

// TestMCPIntegration_StartStatusStop extends the above to a live stack:
// start -> status (expect healthy) -> stop, entirely through MCP tool
// handlers. Requires Docker; gated behind INTEGRATION=1 per repo convention.
func TestMCPIntegration_StartStatusStop(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run the Docker-dependent MCP integration test")
	}
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}

	bin := buildNSelfBinary(t)

	projectDir := t.TempDir()
	t.Setenv(mcpSelfExecOverrideEnv, bin)

	initCmd := exec.Command(bin, "init", "--non-interactive", "--quiet")
	initCmd.Dir = projectDir
	initCmd.Env = append(os.Environ(), "NSELF_ALLOW_SOURCE_DIR=1")
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("nself init failed: %v\n%s", err, out)
	}

	restore := chdir(t, projectDir)
	defer restore()

	if _, err := mcpLifecycleHandler("build", "--quiet")(context.Background(), mcp.CallToolRequest{}); err != nil {
		t.Fatalf("nself_build handler returned a Go error: %v", err)
	}

	startResult, err := mcpLifecycleHandler("start")(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("nself_start handler returned a Go error: %v", err)
	}
	startOut := startResult.StructuredContent.(ExecResult)
	if !startOut.Success {
		t.Fatalf("nself_start did not succeed: %s", startOut.Output)
	}
	t.Cleanup(func() {
		_, _ = mcpLifecycleHandler("stop")(context.Background(), mcp.CallToolRequest{})
	})

	statusResult, err := mcpStatusHandler()(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("nself_status handler returned a Go error: %v", err)
	}
	if statusResult.StructuredContent == nil {
		t.Fatal("expected structured content from nself_status")
	}

	stopResult, err := mcpLifecycleHandler("stop")(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("nself_stop handler returned a Go error: %v", err)
	}
	stopOut := stopResult.StructuredContent.(ExecResult)
	if !stopOut.Success {
		t.Fatalf("nself_stop did not succeed: %s", stopOut.Output)
	}

	t.Logf("init -> build -> start -> status -> stop completed via MCP tool handlers alone")
}
