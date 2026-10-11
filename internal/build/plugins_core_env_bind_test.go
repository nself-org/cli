package build

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestCoreEnvTarget_BindsToDirectKeys: shapes where the first "environment:"
// or "networks:" text in the service is not the service's own key, or the
// service text is not safe to cut (lone CR, anchored target).
func TestCoreEnvTarget_BindsToDirectKeys(t *testing.T) {
	unsafe := map[string]string{
		"lone CR":            "services:\r  w:\r    image: example/w\r    environment:\r      A: value\r    networks: [p]\r  side:\n    image: example/side\n    environment:\n      B: value\n    networks: [p]\nnetworks:\n  p: {}\n",
		"anchored target":    "services:\n  w: &base\n    image: example/w\n    environment:\n      AUTHORED: value\n  side:\n    <<: *base\n    image: example/side\n",
		"nested environment": "services:\n  w:\n    image: example/w\n    x-meta:\n      environment:\n        META: value\n    environment:\n      AUTHORED: value\n",
		"block scalar":       "services:\n  w:\n    image: example/w\n    command: |\n      environment:\n        x\n    environment:\n      AUTHORED: value\n  side:\n    image: example/side\n",
		"nested networks":    "services:\n  w:\n    image: example/w\n    x-meta:\n      networks:\n        - z\n    networks:\n      - p\n  side:\n    image: example/side\n",
	}
	for name, in := range unsafe {
		t.Run(name, func(t *testing.T) {
			pluginDir := t.TempDir()
			writePluginManifestJSON(t, pluginDir, "w", 3777)
			resetWarnOnce()
			var out string
			stderr := captureStderr(t, func() { out = string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "w")) })
			if out != in {
				t.Errorf("fragment must be unchanged:\n%q", out)
			}
			if !strings.Contains(stderr, `plugin "w"`) {
				t.Errorf("expected a warning naming the plugin, got %q", stderr)
			}
		})
	}
}

// TestCoreEnvTarget_NestedEnvironmentAfterDirect: the injection goes into the
// direct environment, not the x-meta one that follows it.
func TestCoreEnvTarget_NestedEnvironmentAfterDirect(t *testing.T) {
	pluginDir := t.TempDir()
	writePluginManifestJSON(t, pluginDir, "w", 3777)
	in := "services:\n  w:\n    image: example/w\n    environment:\n      AUTHORED: value\n    x-meta:\n      environment:\n        META: value\n"
	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "w"))
	var doc struct {
		Services map[string]struct {
			Environment map[string]any `yaml:"environment"`
			Meta        struct {
				Environment map[string]any `yaml:"environment"`
			} `yaml:"x-meta"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	w := doc.Services["w"]
	if w.Environment["PLUGIN_INTERNAL_SECRET"] == nil || w.Environment["AUTHORED"] != "value" {
		t.Errorf("direct environment lacks the core vars:\n%s", out)
	}
	if len(w.Meta.Environment) != 1 || w.Meta.Environment["META"] != "value" {
		t.Errorf("x-meta.environment must be untouched:\n%s", out)
	}
}
