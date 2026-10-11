package build

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// sidecarFirstFragment has a sidecar before the plugin's own service; both
// carry an environment: block and a networks: list.
const sidecarFirstFragment = `services:
  fx-init:
    image: example/init:latest
    environment:
      INIT_ONLY: "1"
    networks:
      - ${DOCKER_NETWORK}
  KEY:
    image: example/fx:latest
    environment:
      DATABASE_URL: ${DATABASE_URL}
    networks:
      - ${DOCKER_NETWORK}
  fx-runner:
    image: example/runner:latest
    environment:
      RUNNER_ONLY: "1"
    networks:
      - ${DOCKER_NETWORK}
networks:
  other:
    driver: bridge
`

// servicesOf decodes a fragment and returns each service's decoded mapping.
func servicesOf(t *testing.T, frag string) map[string]map[string]any {
	t.Helper()
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(frag), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, frag)
	}
	return doc.Services
}

// envKeysOf returns the environment keys of one service (map or list form).
func envKeysOf(t *testing.T, frag, service string) map[string]bool {
	t.Helper()
	keys := map[string]bool{}
	switch env := servicesOf(t, frag)[service]["environment"].(type) {
	case map[string]any:
		for k := range env {
			keys[k] = true
		}
	case []any:
		for _, e := range env {
			k, _, _ := strings.Cut(e.(string), "=")
			keys[k] = true
		}
	}
	return keys
}

func TestCoreEnvTarget_SidecarFirst(t *testing.T) {
	for _, key := range []string{"fx", "nself-fx", "plugin-fx"} {
		t.Run(key, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				pluginDir := t.TempDir()
				writePluginManifestJSON(t, pluginDir, "fx", 3777)
				in := strings.Replace(sidecarFirstFragment, "  KEY:", "  "+key+":", 1)
				out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "fx"))

				for _, side := range []string{"fx-init", "fx-runner"} {
					if !reflect.DeepEqual(servicesOf(t, out)[side], servicesOf(t, in)[side]) {
						t.Errorf("sidecar %s changed:\n%s", side, out)
					}
				}
				own := envKeysOf(t, out, key)
				for _, want := range []string{"SERVICE_NAME", "SERVICE_PORT", "PLUGIN_INTERNAL_SECRET", "DATABASE_URL"} {
					if !own[want] {
						t.Errorf("plugin service lacks %s:\n%s", want, out)
					}
				}
				if own["INIT_ONLY"] || own["RUNNER_ONLY"] {
					t.Errorf("sidecar env leaked into plugin service:\n%s", out)
				}
				if env := servicesOf(t, out)[key]["environment"].(map[string]any); env["DATABASE_URL"] != "${DATABASE_URL}" {
					t.Errorf("fragment's own DATABASE_URL must win: %v", env["DATABASE_URL"])
				}
				if err := assertValidComposeYAML(out); err != nil {
					t.Fatalf("invalid YAML: %v\n%s", err, out)
				}
				if again := string(normalizeComposePluginCoreEnv([]byte(out), pluginDir, "fx")); again != out {
					t.Errorf("not idempotent:\n%s", again)
				}
			})
		})
	}
}

// TestCoreEnvTarget_PortKeyByMode: PORT is injected in v1.5 only (P7-PLUG-17).
func TestCoreEnvTarget_PortKeyByMode(t *testing.T) {
	pluginDir := t.TempDir()
	writePluginManifestJSON(t, pluginDir, "fx", 3777)
	in := strings.Replace(sidecarFirstFragment, "  KEY:", "  fx:", 1)

	compattest.Set(t, true)
	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "fx"))
	if !envKeysOf(t, out, "fx")["PORT"] || envKeysOf(t, out, "fx-init")["PORT"] || envKeysOf(t, out, "fx-runner")["PORT"] {
		t.Errorf("v1.5 must add PORT to the plugin service only:\n%s", out)
	}
	compattest.Set(t, false)
	out = string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "fx"))
	if envKeysOf(t, out, "fx")["PORT"] {
		t.Errorf("v1.4 must not add PORT:\n%s", out)
	}
}

func TestCoreEnvTarget_SingleServiceOtherKey(t *testing.T) {
	pluginDir := t.TempDir()
	writePluginManifestJSON(t, pluginDir, "fx", 3777)
	in := "services:\n  something-else:\n    image: example/x:latest\n    environment:\n      A: \"1\"\n"
	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "fx"))
	if !strings.Contains(out, "SERVICE_NAME: fx") || !strings.Contains(out, "      A: \"1\"\n") {
		t.Errorf("single service must still be injected:\n%s", out)
	}
	if again := string(normalizeComposePluginCoreEnv([]byte(out), pluginDir, "fx")); again != out {
		t.Errorf("not idempotent:\n%s", again)
	}
}

func TestCoreEnvTarget_ManifestNameCandidate(t *testing.T) {
	pluginDir := t.TempDir()
	// Directory fx, manifest name "fxm": the service is named after the manifest.
	writePluginManifestJSON(t, pluginDir, "fx", 3777)
	if err := os.WriteFile(filepath.Join(pluginDir, "fx", "plugin.json"), []byte(`{"name":"fxm","port":3777}`), 0644); err != nil {
		t.Fatal(err)
	}
	in := "services:\n  side:\n    image: example/s:latest\n    environment:\n      S: \"1\"\n  plugin-fxm:\n    image: example/x:latest\n    environment:\n      A: \"1\"\n"
	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "fx"))
	if got, want := out[:strings.Index(out, "  plugin-fxm:")], in[:strings.Index(in, "  plugin-fxm:")]; got != want {
		t.Errorf("sidecar changed:\n%s", got)
	}
	if !strings.Contains(out[strings.Index(out, "  plugin-fxm:"):], "SERVICE_NAME: fx") {
		t.Errorf("manifest-named service not injected:\n%s", out)
	}
}

func TestCoreEnvTarget_MultiServiceNoMatchWarns(t *testing.T) {
	pluginDir := t.TempDir()
	writePluginManifestJSON(t, pluginDir, "fx", 3777)
	in := "services:\n  one:\n    image: a\n    environment:\n      A: \"1\"\n  two:\n    image: b\n    environment:\n      B: \"1\"\n"
	resetWarnOnce()
	var out string
	stderr := captureStderr(t, func() { out = string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "fx")) })
	if out != in {
		t.Errorf("fragment must be unchanged:\n%s", out)
	}
	for _, want := range []string{`"fx"`, "docker-compose.plugin.yml", "nself-fx", "plugin-fx", "not injected"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("warning lacks %q: %q", want, stderr)
		}
	}
}

// TestCoreEnvTarget_NewBlockGoesInTarget: the target has no environment: but a
// networks: list, and the sidecar before it has a networks: list too.
func TestCoreEnvTarget_NewBlockGoesInTarget(t *testing.T) {
	pluginDir := t.TempDir()
	writePluginManifestJSON(t, pluginDir, "fx", 3777)
	in := "services:\n  fx-init:\n    image: example/init\n    networks:\n      - ${DOCKER_NETWORK}\n  fx:\n    image: example/fx\n    networks:\n      - ${DOCKER_NETWORK}\n"
	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "fx"))
	if !reflect.DeepEqual(servicesOf(t, out)["fx-init"], servicesOf(t, in)["fx-init"]) {
		t.Errorf("sidecar changed:\n%s", out)
	}
	if !envKeysOf(t, out, "fx")["SERVICE_NAME"] || len(envKeysOf(t, out, "fx-init")) != 0 {
		t.Errorf("new env block must land in fx only:\n%s", out)
	}
	if err := assertValidComposeYAML(out); err != nil {
		t.Fatalf("invalid YAML: %v\n%s", err, out)
	}
	if again := string(normalizeComposePluginCoreEnv([]byte(out), pluginDir, "fx")); again != out {
		t.Errorf("not idempotent:\n%s", again)
	}
}

func TestCoreEnvTarget_CRLF(t *testing.T) {
	pluginDir := t.TempDir()
	writePluginManifestJSON(t, pluginDir, "fx", 3777)
	in := "services:\r\n  fx-init:\r\n    image: a\r\n    environment:\r\n      I: \"1\"\r\n  fx:\r\n    image: b\r\n    environment:\r\n      A: \"1\"\r\n"
	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "fx"))
	if !strings.HasPrefix(out, in[:strings.Index(in, "  fx:")]) {
		t.Errorf("sidecar changed under CRLF:\n%q", out)
	}
	if !strings.Contains(out[strings.Index(out, "  fx:"):], "SERVICE_NAME") {
		t.Errorf("CRLF target not injected:\n%q", out)
	}
}
