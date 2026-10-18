package commands

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/doctor"
)

// doctorPlugin installs a fixture plugin whose binary is the shell script
// body. withDoctor controls whether the manifest lists a `doctor` subcommand.
func doctorPlugin(t *testing.T, dir, slug string, withDoctor bool, body string) {
	t.Helper()
	subs := []any{map[string]any{"name": "sub"}}
	if withDoctor {
		subs = append(subs, map[string]any{"name": "doctor", "side_effect": "read", "json": "envelope"})
	}
	m := map[string]any{
		"manifest_version": 2, "name": slug, "version": "1.0.0", "description": "fixture",
		"license": "free", "licenseType": "free", "pluginType": "cli", "category": "infrastructure",
		"status": "stable", "tier": "free", "maturity": "implemented", "minNselfVersion": "1.5.0",
		"service": map[string]any{"kind": "cli"}, "requires": map[string]any{"nself": ">=1.5.0"},
		"binaryName": "nself-" + slug,
		"commands":   map[string]any{"command": slug, "binary": "nself-" + slug, "subcommands": subs},
	}
	b, _ := json.Marshal(m)
	if err := os.MkdirAll(filepath.Join(dir, slug), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, slug, "plugin.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "bin", "nself-"+slug), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

const okEnvelope = `{"schema_version":"1","command":"doctor","data":{"checks":[` +
	`{"id":"FX-D001","name":"tool","status":"pass","message":"tool found","remediation":""},` +
	`{"id":"FX-D002","name":"repo","status":"fail","message":"repo broken","remediation":"fix it","subject":"main"},` +
	`{"id":"FX-D003","name":"odd","status":"weird","message":"m","remediation":""}` +
	`],"summary":{"pass":1,"warn":0,"fail":1}}}`

func pluginHome(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stubs need a POSIX shell")
	}
	dir := filepath.Join(t.TempDir(), "plugins")
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	return dir
}

func TestExternalDoctorsFold(t *testing.T) {
	dir := pluginHome(t)
	doctorPlugin(t, dir, "okdoc", true, "echo '"+okEnvelope+"'; exit 10")
	got := doctor.ExternalChecks(context.Background(), ".")
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	for _, r := range got {
		if r.Section != "plugin:okdoc" {
			t.Fatalf("section %q", r.Section)
		}
	}
	if got[0].Name != "FX-D001" || got[0].Status != "pass" || got[0].Message != "tool found" {
		t.Fatalf("row 0 %+v", got[0])
	}
	if got[1].Status != "fail" || got[1].FixCmd != "fix it" || got[1].Message != "repo broken (main)" {
		t.Fatalf("row 1 %+v", got[1])
	}
	if got[2].Status != "warn" {
		t.Fatalf("unknown status must map to warn: %+v", got[2])
	}
}

func TestExternalDoctorsAbsent(t *testing.T) {
	dir := pluginHome(t)
	doctorPlugin(t, dir, "nodoc", false, "echo '"+okEnvelope+"'")
	if got := doctor.ExternalChecks(context.Background(), "."); len(got) != 0 {
		t.Fatalf("a plugin without a doctor subcommand must add nothing: %+v", got)
	}
	empty := filepath.Join(t.TempDir(), "none")
	t.Setenv("NSELF_PLUGIN_DIR", empty)
	if got := doctor.ExternalChecks(context.Background(), "."); len(got) != 0 {
		t.Fatalf("no plugin dir: %+v", got)
	}
}

func TestExternalDoctorsArgv(t *testing.T) {
	dir := pluginHome(t)
	// Prints a valid envelope only when called with exactly `doctor --json`.
	doctorPlugin(t, dir, "argdoc", true,
		`[ "$*" = "doctor --json" ] || { echo "bad argv: $*"; exit 2; }; echo '`+okEnvelope+`'`)
	if got := doctor.ExternalChecks(context.Background(), "."); len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
}

func TestExternalDoctorsExitCodes(t *testing.T) {
	dir := pluginHome(t)
	doctorPlugin(t, dir, "warndoc", true, "echo '"+okEnvelope+"'; exit 12")
	doctorPlugin(t, dir, "crashdoc", true, "echo '"+okEnvelope+"'; exit 3")
	got := doctor.ExternalChecks(context.Background(), ".")
	var crash []doctor.CheckResult
	n := 0
	for _, r := range got {
		switch r.Section {
		case "plugin:warndoc":
			n++
		case "plugin:crashdoc":
			crash = append(crash, r)
		}
	}
	if n != 3 {
		t.Fatalf("exit 12 maps to the plugin's checks: %+v", got)
	}
	if len(crash) != 1 || crash[0].Status != "warn" || !strings.Contains(crash[0].Message, "exited 3") {
		t.Fatalf("exit 3 is one warning: %+v", crash)
	}
}

func TestExternalDoctorsSlowInvalid(t *testing.T) {
	dir := pluginHome(t)
	doctorPlugin(t, dir, "slowdoc", true, "exec sleep 30")
	doctorPlugin(t, dir, "baddoc", true, "echo 'not json {'")
	doctorPlugin(t, dir, "gooddoc", true, "echo '"+okEnvelope+"'")
	old := externalDoctorTimeout
	externalDoctorTimeout = 300 * time.Millisecond
	t.Cleanup(func() { externalDoctorTimeout = old })

	start := time.Now()
	got := doctor.ExternalChecks(context.Background(), ".")
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("deep doctor took %s; the slow plugin must be cut off", d)
	}
	by := map[string][]doctor.CheckResult{}
	for _, r := range got {
		by[r.Section] = append(by[r.Section], r)
	}
	slow, bad := by["plugin:slowdoc"], by["plugin:baddoc"]
	if len(slow) != 1 || slow[0].Status != "warn" || !strings.Contains(slow[0].Message, "slowdoc") ||
		!strings.Contains(slow[0].Message, "did not finish") {
		t.Fatalf("slow: %+v", slow)
	}
	if len(bad) != 1 || bad[0].Status != "warn" || !strings.Contains(bad[0].Message, "baddoc") ||
		!strings.Contains(bad[0].Message, "no v1 envelope") {
		t.Fatalf("invalid: %+v", bad)
	}
	if len(by["plugin:gooddoc"]) != 3 {
		t.Fatalf("a healthy plugin is unaffected: %+v", by["plugin:gooddoc"])
	}
}

func TestExternalDoctorsEscapesPluginText(t *testing.T) {
	r := mapPluginCheck("plugin:x", pluginDoctorCheck{ID: "A\x1b[31m", Status: "pass", Message: "m\x1b]0;t\x07", Remediation: "r\n"})
	for _, s := range []string{r.Name, r.Message, r.FixCmd} {
		if strings.ContainsAny(s, "\x1b\x07\n") {
			t.Fatalf("control character leaked: %q", s)
		}
	}
}
