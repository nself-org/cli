package commands

// Purpose: a disabled builtin family (nself remove admin) must not break the
// v1.5 --json surface (D-0863). Runs the real binary: remove admin, then
// version --json must answer with one success envelope, and the family must
// stay unmounted (E407).
// Constraints: child process only; HOME is the test's temp dir.

import (
	"strings"
	"testing"
)

func TestDisabledBuiltinFamilyKeepsJSONSurface(t *testing.T) {
	proj := cfgFixtureProject(t)
	rm := runPilot(t, proj, true, nil, "remove", "admin")
	if rm.code != 0 {
		t.Fatalf("remove admin: exit %d\n%s%s", rm.code, rm.stdout, rm.stderr)
	}
	ver := runPilot(t, proj, true, nil, "version", "--json")
	if ver.code != 0 {
		t.Fatalf("version --json after remove admin: exit %d\n%s%s", ver.code, ver.stdout, ver.stderr)
	}
	validateEnvelope(t, ver.stdout, "version")
	if strings.Contains(ver.stdout, "command registry") {
		t.Fatalf("registry build error leaked into the envelope:\n%s", ver.stdout)
	}
	// The disabled family stays unmounted: E407, not a registry problem.
	adm := runPilot(t, proj, true, nil, "admin", "--json")
	if adm.code == 0 || !strings.Contains(adm.stdout, `"E407"`) {
		t.Fatalf("admin --json after remove admin: exit %d, want E407\n%s%s", adm.code, adm.stdout, adm.stderr)
	}
}

// The machine registry (nself mcp, nself invoke) is built apart from the
// runtime one; it must mount a disabled family too (D-0863), or the family's
// canon entries are all unresolved and mcp exits before serving.
func TestDisabledBuiltinFamilyKeepsMachineRegistry(t *testing.T) {
	proj := cfgFixtureProject(t)
	rm := runPilot(t, proj, true, nil, "remove", "admin")
	if rm.code != 0 {
		t.Fatalf("remove admin: exit %d\n%s%s", rm.code, rm.stdout, rm.stderr)
	}
	// stdin is closed, so a server that built its tools exits cleanly.
	mcp := runPilot(t, proj, true, nil, "mcp")
	out := mcp.stdout + mcp.stderr
	if mcp.code != 0 || strings.Contains(out, "machine registry") || strings.Contains(out, "unresolved") {
		t.Fatalf("mcp after remove admin: exit %d, want a clean start\n%s", mcp.code, out)
	}
}
