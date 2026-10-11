package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// splitService returns the text of one service block of a fragment built in
// the shape of sidecarFirstFragment (two-space service keys).
func splitService(t *testing.T, frag, key string) string {
	t.Helper()
	start := strings.Index(frag, "\n  "+key+":\n")
	if start < 0 {
		t.Fatalf("service %s not found:\n%s", key, frag)
	}
	rest := frag[start+1:]
	next := len(rest)
	for _, m := range []string{"\n  fx", "\n  nself-", "\n  plugin-", "\n  KEY", "\nnetworks:"} {
		if i := strings.Index(rest[1:], m); i >= 0 && i+1 < next {
			next = i + 1
		}
	}
	return rest[:next]
}

func TestCoreEnvTarget_SidecarFirst(t *testing.T) {
	for _, key := range []string{"fx", "nself-fx", "plugin-fx"} {
		t.Run(key, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				pluginDir := t.TempDir()
				writePluginManifestJSON(t, pluginDir, "fx", 3777)
				in := strings.Replace(sidecarFirstFragment, "  KEY:", "  "+key+":", 1)
				out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "fx"))

				if got, want := splitService(t, out, "fx-init"), splitService(t, in, "fx-init"); got != want {
					t.Errorf("sidecar fx-init changed:\n%s", got)
				}
				if got, want := splitService(t, out, "fx-runner"), splitService(t, in, "fx-runner"); got != want {
					t.Errorf("sidecar fx-runner changed:\n%s", got)
				}
				own := splitService(t, out, key)
				for _, want := range []string{"SERVICE_NAME: fx", `SERVICE_PORT: "3777"`, "PLUGIN_INTERNAL_SECRET:"} {
					if !strings.Contains(own, want) {
						t.Errorf("plugin service lacks %q:\n%s", want, own)
					}
				}
				if strings.Contains(own, "INIT_ONLY") || strings.Contains(own, "RUNNER_ONLY") {
					t.Errorf("sidecar env leaked into plugin service:\n%s", own)
				}
				if strings.Count(own, "DATABASE_URL:") != 1 {
					t.Errorf("fragment's own DATABASE_URL must win, once:\n%s", own)
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
	own := splitService(t, string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "fx")), "fx")
	if !strings.Contains(own, "\n      PORT: \"3777\"") {
		t.Errorf("v1.5 must add PORT to the plugin service:\n%s", own)
	}
	compattest.Set(t, false)
	own = splitService(t, string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "fx")), "fx")
	if strings.Contains(own, "\n      PORT:") {
		t.Errorf("v1.4 must not add PORT:\n%s", own)
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
	if got, want := splitService(t, out, "fx-init"), splitService(t, in, "fx-init"); got != want {
		t.Errorf("sidecar changed:\n%s", got)
	}
	own := splitService(t, out, "fx")
	if !strings.Contains(own, "    environment:\n") || !strings.Contains(own, "SERVICE_NAME: fx") {
		t.Errorf("new env block must land in fx:\n%s", out)
	}
	if err := assertValidComposeYAML(out); err != nil {
		t.Fatalf("invalid YAML: %v\n%s", err, out)
	}
	if again := string(normalizeComposePluginCoreEnv([]byte(out), pluginDir, "fx")); again != out {
		t.Errorf("not idempotent:\n%s", again)
	}
}

func TestCoreEnvTarget_CRLFAndUnparseable(t *testing.T) {
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
	// Unparseable fragments keep the whole-content behaviour.
	bad := "services: [\n  environment:\n    A: 1\n"
	if s, e, ok := pluginServiceRange([]byte(bad), "fx", ""); !ok || s != 0 || e != len(bad) {
		t.Errorf("unparseable: got %d %d %v", s, e, ok)
	}
}
