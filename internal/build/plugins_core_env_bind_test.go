package build

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestCoreEnvTarget_BindsToDirectKeys: shapes where the match the rewrite
// would act on is not at the service's own key (a nested key follows a direct
// key no regex matches), or the service text is not safe to cut (lone CR,
// anchored target). All stay unchanged with a warning.
func TestCoreEnvTarget_BindsToDirectKeys(t *testing.T) {
	unsafe := map[string]string{
		"lone CR": "services:\r  w:\r    image: example/w\r    environment:\r      A: value\r    networks: [p]\r  side:\n    image: example/side\n    environment:\n      B: value\n    networks: [p]\nnetworks:\n  p: {}\n",
		"empty-list environment nested after a direct map": "services:\n  w:\n    image: example/w\n    environment:\n      AUTHORED: value\n    x-meta:\n      environment: []\n",
		"flow environment then nested block":               "services:\n  w:\n    image: example/w\n    environment: {}\n    x-meta:\n      environment:\n        META: value\n",
		"commented direct networks then nested list":       "services:\n  w:\n    image: example/w\n    networks:\n      # c\n      - p\n    x-meta:\n      networks:\n        - z\n  side:\n    image: example/side\n",
		"anchored target": "services:\n  w: &base\n    image: example/w\n    environment:\n      AUTHORED: value\n  side:\n    <<: *base\n    image: example/side\n",
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

// decodeW decodes a fragment's service w with its x-meta extension.
func decodeW(t *testing.T, frag string) (env map[string]any, metaEnv map[string]any, metaNets []any) {
	t.Helper()
	var doc struct {
		Services map[string]struct {
			Environment map[string]any `yaml:"environment"`
			Meta        struct {
				Environment map[string]any `yaml:"environment"`
				Networks    []any          `yaml:"networks"`
			} `yaml:"x-meta"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(frag), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, frag)
	}
	w := doc.Services["w"]
	return w.Environment, w.Meta.Environment, w.Meta.Networks
}

// TestCoreEnvTarget_AnchoredPositive: the rewrite lands in the direct key
// even when a nested key with the same text sits elsewhere in the service.
func TestCoreEnvTarget_AnchoredPositive(t *testing.T) {
	cases := map[string]struct {
		in          string
		wantMetaEnv int
		wantNets    int
	}{
		"direct environment after a block scalar": {"services:\n  w:\n    image: example/w\n    command: |\n      environment:\n        x\n    environment:\n      AUTHORED: value\n", 0, 0},
		"direct environment before nested one":    {"services:\n  w:\n    image: example/w\n    environment:\n      AUTHORED: value\n    x-meta:\n      environment:\n        META: value\n", 1, 0},
		"no environment, networks, nested list":   {"services:\n  w:\n    image: example/w\n    networks:\n      - p\n    x-meta:\n      networks:\n        - z\n", 0, 1},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			pluginDir := t.TempDir()
			writePluginManifestJSON(t, pluginDir, "w", 3777)
			out := string(normalizeComposePluginCoreEnv([]byte(c.in), pluginDir, "w"))
			env, metaEnv, metaNets := decodeW(t, out)
			if env["PLUGIN_INTERNAL_SECRET"] == nil || env["SERVICE_NAME"] != "w" {
				t.Errorf("w.environment lacks the core vars:\n%s", out)
			}
			if len(metaEnv) != c.wantMetaEnv || len(metaNets) != c.wantNets {
				t.Errorf("x-meta changed:\n%s", out)
			}
			if strings.Count(out, "\n    environment:") != 1 {
				t.Errorf("want exactly one direct environment key:\n%s", out)
			}
			if again := string(normalizeComposePluginCoreEnv([]byte(out), pluginDir, "w")); again != out {
				t.Errorf("not idempotent:\n%s", again)
			}
		})
	}
}
