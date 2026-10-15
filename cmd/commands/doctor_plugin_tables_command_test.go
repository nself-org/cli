package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

// Exercise the actual command so a disconnected diagnostic cannot pass.
func TestDoctorReportsDeferredPluginTables(t *testing.T) {
	compattest.Set(t, true)
	pluginDir := readinessProject(t, "fine", 1, 1)
	cwd, _ := os.Getwd()
	if err := os.WriteFile(filepath.Join(cwd, ".env"), []byte("PROJECT_NAME=rdy\nENV=dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV", "prod")
	oldLoad := bundleLoadConfig
	var loadedSelector string
	bundleLoadConfig = func(dir string) (*config.Config, error) {
		loadedSelector = os.Getenv("ENV")
		return oldLoad(dir)
	}
	t.Cleanup(func() { bundleLoadConfig = oldLoad })
	t.Setenv("NSELF_PLUGIN_DIR", pluginDir)
	t.Setenv("PATH", t.TempDir()) // no host Docker/git or production services
	_, exports := realTracking(t, nil, `{"sources":[{"name":"default","tables":[]}]}`, nil)
	for _, name := range []string{"json", "quick", "fix", "deep"} {
		f := doctorCmd.Flags().Lookup(name)
		before := f.Value.String()
		t.Cleanup(func() { _ = f.Value.Set(before) })
		value := "false"
		if name == "json" || name == "quick" {
			value = "true"
		}
		if err := f.Value.Set(value); err != nil {
			t.Fatal(err)
		}
	}
	output, _ := captureStdout(t, func() error { return doctorCmd.RunE(doctorCmd, nil) })
	if loadedSelector != "prod" {
		t.Fatalf("doctor lost process ENV=prod after earlier checks loaded ENV=dev: %q", loadedSelector)
	}
	// Config loaders may print informational lines before the JSON report.
	start := strings.Index(output, "{\n")
	if start < 0 {
		t.Fatalf("no doctor report in %q", output)
	}
	var envelope struct {
		Data doctorReport `json:"data"`
	}
	if err := json.Unmarshal([]byte(output[start:]), &envelope); err != nil {
		t.Fatalf("report: %v\n%s", err, output)
	}
	report := envelope.Data
	for _, r := range report.Checks {
		if r.Name == "Plugin tables: fine" {
			if r.Status != "fail" || !strings.Contains(r.Message, "public.np_fine_items") || !strings.Contains(r.Detail, "nself doctor --fix") || *exports != 1 {
				t.Fatalf("diagnostic = %+v, exports = %d", r, *exports)
			}
			return
		}
	}
	t.Fatalf("doctor omitted deferred plugin tables: %+v", report.Checks)
}
