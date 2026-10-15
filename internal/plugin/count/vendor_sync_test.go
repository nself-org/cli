package count

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Purpose: exercise the legacy shim without a real sibling checkout.
// Inputs: a stub generator that records arguments; outputs: read-only calls.
// Constraints: all fixture writes remain inside the test's temporary directory.
func TestCountsShimReadOnly(t *testing.T) {
	shim, err := os.ReadFile("../../../scripts/plugin-counts.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"--json"}, {"--check"}, {"--check", "--free-only"}, {"--init-releases"}, {"--free-only"}} {
		t.Run(strings.Join(args, ","), func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range []string{"cli/scripts", "plugins/scripts"} {
				if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
					t.Fatal(err)
				}
			}
			shimPath := filepath.Join(root, "cli/scripts/plugin-counts.sh")
			if err := os.WriteFile(shimPath, shim, 0755); err != nil {
				t.Fatal(err)
			}
			stub := "#!/bin/bash\nprintf '%s\\n' \"$@\"\n"
			if err := os.WriteFile(filepath.Join(root, "plugins/scripts/plugin-counts.sh"), []byte(stub), 0755); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command("bash", append([]string{shimPath}, args...)...).CombinedOutput()
			if len(args) > 0 && (args[0] == "--init-releases" || args[0] == "--free-only") {
				if err == nil {
					t.Fatal("option without a read-only mode was forwarded to generator")
				}
				return
			}
			if err != nil {
				t.Fatalf("shim: %v: %s", err, out)
			}
			want := "--json\n"
			if len(args) > 0 {
				want = strings.Join(args, "\n") + "\n"
			}
			if string(out) != want {
				t.Fatalf("generator args = %q, want %q", out, want)
			}
		})
	}
}

// Purpose: run the workflow's real comparison against isolated JSON fixtures.
// Inputs: provenance-only and semantic drift; outputs: success and failure.
// Constraints: no network or sibling checkout is required.
func TestCountsVendorWorkflowComparison(t *testing.T) {
	workflow, err := os.ReadFile("../../../.github/workflows/counts-vendor-sync.yml")
	if err != nil {
		t.Fatal(err)
	}
	const start = "          python3 - <<'PY'\n"
	_, script, ok := strings.Cut(string(workflow), start)
	if !ok {
		t.Fatal("workflow Python comparison missing")
	}
	script, _, ok = strings.Cut(script, "          PY")
	if !ok {
		t.Fatal("workflow Python terminator missing")
	}
	script = strings.ReplaceAll(script, "          ", "")
	for _, drift := range []bool{false, true} {
		root := t.TempDir()
		for _, dir := range []string{"internal/plugin/count", ".plugins-canonical"} {
			if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
				t.Fatal(err)
			}
		}
		for i, path := range []string{"internal/plugin/count/counts.json", ".plugins-canonical/counts.json"} {
			advertised := 171
			if drift && i == 1 {
				advertised++
			}
			data, err := json.Marshal(map[string]any{"advertised": advertised, "generated_from": map[string]any{"bundles_ref": i, "generated_at": i, "plugins_ref": i}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, path), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command("python3", "-c", script)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if (err != nil) != drift {
			t.Errorf("semantic drift=%v: comparison error=%v: %s", drift, err, out)
		}
	}
}
