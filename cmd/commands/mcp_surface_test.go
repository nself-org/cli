package commands

// Purpose: prove the P7-SHIP-02 product gaps of `nself mcp` are closed: the
// server starts without a project and lists init plus the read-only tools,
// lists everything after a project exists, lists a plugin's tools after the
// plugin is installed (with tools/list_changed each time), bounds calls by
// --timeout, and takes `deploy promote <source> <target>`.
// Constraints: each case runs in a fresh copy of this test binary (the
// machine registry mutates the shared command tree); the child is the stub
// from internal/invoke/testdata/stubcli; no network, no sleeps.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
)

// waitListChanged reads until the server has sent tools/list_changed, with
// ping round trips (a notification and a response may interleave).
func waitListChanged(t *testing.T, sess *stdioSession) {
	t.Helper()
	for i := 0; i < 20; i++ {
		if slices.Contains(sess.notes, "notifications/tools/list_changed") {
			sess.notes = nil
			return
		}
		sess.call("ping", `{}`)
	}
	t.Fatalf("no notifications/tools/list_changed; notifications seen: %v", sess.notes)
}

func TestMCPSurfaceWithoutProjectThenRefresh(t *testing.T) {
	if !inFreshProcess(t, "TestMCPSurfaceWithoutProjectThenRefresh") {
		return
	}
	home := stdioFixture(t, true)
	proj := filepath.Join(home, "proj")
	if err := os.Remove(filepath.Join(proj, ".env")); err != nil {
		t.Fatal(err)
	}
	s, err := newMCPServer("stdio", true, 0)
	if err != nil {
		t.Fatalf("the server must start without a project: %v", err)
	}
	sess := startStdio(t, s)
	listed := toolsOf(t, sess)
	if _, ok := listed["nself_init"]; !ok {
		t.Fatal("nself_init is not listed without a project")
	}
	if _, ok := listed["nself_status"]; !ok {
		t.Error("the read-only nself_status is not listed without a project")
	}
	for _, name := range []string{"nself_build", "nself_start", "nself_reset", "nself_deploy_promote"} {
		if _, ok := listed[name]; ok {
			t.Errorf("%s is listed without a project", name)
		}
	}
	for name, raw := range listed {
		if name != "nself_init" && !strings.Contains(raw, `"readOnlyHint":true`) {
			t.Errorf("%s is listed without a project but is not read-only", name)
		}
	}

	// init creates the project (the stub only answers; the test writes what init writes).
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if res := callTool(t, sess, "nself_init", `{}`); res.IsError {
		t.Fatalf("nself_init: %+v", res)
	}
	waitListChanged(t, sess)
	listed = toolsOf(t, sess)
	for _, name := range []string{"nself_build", "nself_start", "nself_deploy_promote"} {
		if _, ok := listed[name]; !ok {
			t.Errorf("%s is not listed after the project exists", name)
		}
	}

	// A plugin installed during the session: its tools appear after the next call.
	if _, ok := listed["nself_sentry_status"]; ok {
		t.Fatal("the fixture plugin is listed before it is installed")
	}
	installSentryFixture(t, filepath.Join(home, "plugins"))
	if res := callTool(t, sess, "nself_status", `{}`); res.IsError {
		t.Fatalf("nself_status: %+v", res)
	}
	waitListChanged(t, sess)
	if _, ok := toolsOf(t, sess)["nself_sentry_status"]; !ok {
		t.Fatal("the plugin installed in this session is not listed")
	}
	// Nothing changed: no rebuild, no notification.
	callTool(t, sess, "nself_status", `{}`)
	sess.call("ping", `{}`)
	if slices.Contains(sess.notes, "notifications/tools/list_changed") {
		t.Error("tools/list_changed was sent although nothing changed")
	}
	t.Log("CHECKED-TestMCPSurfaceWithoutProjectThenRefresh")
}

func TestMCPTimeoutBoundsCalls(t *testing.T) {
	if !inFreshProcess(t, "TestMCPTimeoutBoundsCalls") {
		return
	}
	stdioFixture(t, true)
	t.Setenv("STUBCLI_MODE", "sleep")
	s, err := newMCPServer("stdio", true, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	res := callTool(t, startStdio(t, s), "nself_status", `{}`)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "E423") || !strings.Contains(res.Content[0].Text, "within 1s") {
		t.Fatalf("a call longer than --timeout: %+v", res)
	}
	t.Log("CHECKED-TestMCPTimeoutBoundsCalls")
}

func TestMCPTimeoutFlag(t *testing.T) {
	f := mcpCmd.Flags().Lookup("timeout")
	if f == nil || f.DefValue != invoke.DefaultTimeout.String() {
		t.Fatalf("--timeout: %+v", f)
	}
	t.Cleanup(func() { _ = f.Value.Set(f.DefValue); f.Changed = false })
	if err := mcpCmd.Flags().Set("timeout", "0s"); err != nil {
		t.Fatal(err)
	}
	if d := errs.Describe(runMCPServe(mcpCmd, nil)); d == nil || d.Code != "E401" {
		t.Fatalf("--timeout 0s: want E401, got %v", d)
	}
}

func TestMCPDeployPromoteTakesSourceAndTarget(t *testing.T) {
	if !inFreshProcess(t, "TestMCPDeployPromoteTakesSourceAndTarget") {
		return
	}
	stdioFixture(t, true)
	reg, err := machineRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cmd, ok := reg.Lookup("deploy promote")
	if !ok {
		t.Fatal("deploy promote is not in the machine registry")
	}
	argv, err := invoke.BuildArgvFor(cmd, invoke.Request{Args: []string{"qa", "prod"}, Flags: map[string]any{"dry-run": true}}, invoke.TransportMCP)
	if err != nil {
		t.Fatalf("deploy promote qa prod: %v", err)
	}
	if got := strings.Join(argv[len(argv)-2:], " "); got != "qa prod" {
		t.Errorf("argv %q does not end with the two environments", argv)
	}
	if _, err := invoke.BuildArgvFor(cmd, invoke.Request{Args: []string{"qa", "prod", "extra"}}, invoke.TransportMCP); err == nil {
		t.Error("three args were accepted")
	}
	t.Log("CHECKED-TestMCPDeployPromoteTakesSourceAndTarget")
}
