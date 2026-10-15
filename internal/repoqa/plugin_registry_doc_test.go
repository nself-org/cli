package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin"
)

// TestPluginRegistryEnvDocumentation keeps the operator-facing registry override
// discoverable, with the runtime default and both command families documented.
// Inputs: the environment variable wiki table and plugin.DefaultRegistryURL.
// Outputs: a failure when the row is missing or its contract drifts.
// Constraints: read-only; no network or plugin installation (SPORT: D-0659).
func TestPluginRegistryEnvDocumentation(t *testing.T) {
	path := filepath.Join(repoRoot(t), ".github", "wiki", "Config-Env-Vars.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		cells := strings.Split(line, "|")
		if len(cells) != 7 || strings.TrimSpace(cells[1]) != "`NSELF_PLUGIN_REGISTRY`" {
			continue
		}
		if strings.TrimSpace(cells[2]) != "string" {
			t.Errorf("registry override must be documented as a string: %s", line)
		}
		if strings.Trim(strings.TrimSpace(cells[3]), "`") != plugin.DefaultRegistryURL {
			t.Errorf("registry default must match %s: %s", plugin.DefaultRegistryURL, line)
		}
		for _, want := range []string{"override", "plugin", "update"} {
			if !strings.Contains(strings.ToLower(cells[5]), want) {
				t.Errorf("registry description must mention %q: %s", want, line)
			}
		}
		return
	}
	t.Fatal("Config-Env-Vars.md lacks the NSELF_PLUGIN_REGISTRY table row")
}
