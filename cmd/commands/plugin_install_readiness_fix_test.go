package commands

// plugin_install_readiness_fix_test.go — P7-PLUG-19 fix round 1: setup problems
// of the plugin-install readiness check are returned in v1.5 and warned in v1.4
// (only "not in a project" is silent), including a boot plugin with no port.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestPluginInstallReadinessBrokenManifest: an unparseable plugin.json is not
// swallowed.
func TestPluginInstallReadinessBrokenManifest(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		pluginDir := readinessProject(t, "stuck", 0, 1)
		if err := os.WriteFile(filepath.Join(pluginDir, "stuck", "plugin.json"), []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		stubReadinessSeams(t, true)
		c, stderr := readinessCmd()
		err := pluginInstallReadiness(c, pluginDir, []string{"stuck"})
		if compat.V15() {
			if err == nil || !strings.Contains(err.Error(), "stuck") {
				t.Fatalf("v1.5: err = %v, want an error naming the plugin", err)
			}
			return
		}
		if err != nil || !strings.Contains(stderr.String(), "warning:") || !strings.Contains(stderr.String(), "stuck") {
			t.Fatalf("v1.4: err = %v stderr = %q, want a warning naming the plugin", err, stderr.String())
		}
	})
}

// TestPluginInstallReadinessNoPort: a boot-migrations plugin with no port
// cannot be probed: v1.5 fails, v1.4 warns (same rule as the bundle flow).
func TestPluginInstallReadinessNoPort(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		pluginDir := readinessProject(t, "portless", 0, 1)
		path := filepath.Join(pluginDir, "portless", "plugin.json")
		raw, _ := os.ReadFile(path)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		delete(m, "port")
		if svc, ok := m["service"].(map[string]any); ok {
			delete(svc, "port")
		}
		out, _ := json.Marshal(m)
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
		stubReadinessSeams(t, true)
		c, stderr := readinessCmd()
		err := pluginInstallReadiness(c, pluginDir, []string{"portless"})
		if compat.V15() {
			if err == nil || !strings.Contains(err.Error(), "portless") {
				t.Fatalf("v1.5: err = %v, want an error naming the plugin", err)
			}
			return
		}
		if err != nil || !strings.Contains(stderr.String(), "portless") {
			t.Fatalf("v1.4: err = %v stderr = %q, want a warning naming the plugin", err, stderr.String())
		}
	})
}
