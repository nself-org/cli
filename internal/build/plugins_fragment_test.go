package build

// Purpose: P7-PLUG-17 acceptance tests for compose fragments: a compose
// plugin with no fragment (E128), network attachment and rewrite, a foreign
// external network (E129), PORT, and the E130 registration. Fixtures are
// copied from licensed fragments at the forge basis (plugins-pro/paid:
// activity-feed has no networks, nself-audit uses ${DOCKER_NETWORK:-nself_network}).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

const fragNoNetworks = `services:
  activity-feed:
    build: .
    image: nself/activity-feed:latest
    restart: unless-stopped
    ports:
      - "3209:3209"
    environment:
      DATABASE_URL: ${DATABASE_URL}
`

const fragDefaultNetwork = `# nself-audit compose fragment
version: "3.8"

services:
  nself-audit:
    image: nself-audit:latest
    container_name: nself-audit
    ports:
      - "3843:3843"
    networks:
      - ${DOCKER_NETWORK:-nself_network}
    depends_on:
      - postgres
`

const fragProjectExternal = `services:
  browser:
    image: nself/nself-browser:latest
    networks:
      - nself_network
networks:
  nself_network:
    external: true
    name: ${COMPOSE_PROJECT_NAME:-nself}_network
`

const fragForeign = `services:
  spy:
    image: nself/spy:latest
    networks:
      - proxy
networks:
  proxy:
    external: true
    name: traefik_proxy
`

// installPlugin plants <dir>/<name>/ with plugin.json and (when fragment is
// not empty) docker-compose.plugin.yml; extra files are created empty.
func installPlugin(t *testing.T, dir, name, manifest, fragment string, extra ...string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, name, "plugin.json"), manifest)
	if fragment != "" {
		writeFile(t, filepath.Join(dir, name, pluginComposeFilename), fragment)
	}
	for _, f := range extra {
		writeFile(t, filepath.Join(dir, name, f), "")
	}
}

func readFragment(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name, pluginComposeFilename))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestFragmentLessComposePluginE128: a plugin that declares a compose service
// but ships no fragment fails the build (v1.5) or warns (v1.4); plugins that
// run no container, disabled plugins and unreadable manifests stay silent.
func TestFragmentLessComposePluginE128(t *testing.T) {
	cases := []struct {
		name, manifest string
		extra          []string
		disabled       bool
		compose        bool // a compose plugin missing its fragment
	}{
		{"v2 compose", `{"manifest_version":2,"name":"p","service":{"kind":"compose"}}`, nil, false, true},
		{"v1 dockerfile and port", `{"name":"p","port":3800}`, []string{"Dockerfile"}, false, true},
		{"v2 cli", `{"manifest_version":2,"name":"p","service":{"kind":"cli"}}`, nil, false, false},
		{"v2 library", `{"manifest_version":2,"name":"p","service":{"kind":"library"}}`, nil, false, false},
		{"v1 port without dockerfile", `{"name":"p","port":3800}`, nil, false, false},
		{"v1 dockerfile without port", `{"name":"p"}`, []string{"Dockerfile"}, false, false},
		{"unreadable manifest", `{not json`, []string{"Dockerfile"}, false, false},
		{"disabled compose plugin", `{"manifest_version":2,"name":"p","service":{"kind":"compose"}}`, []string{".disabled"}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("v1.5", func(t *testing.T) {
				compattest.Set(t, true)
				dir := t.TempDir()
				installPlugin(t, dir, "p", tc.manifest, "", tc.extra...)
				files, err := discoverPluginComposeFilesFx(writeEffects{}, newDiskSink(dir), dir, dir)
				var ce *errs.CLIError
				if tc.compose {
					if !errors.As(err, &ce) || ce.Code != "E128" || !strings.Contains(ce.What, `"p"`) {
						t.Fatalf("want E128 naming the plugin, got %v", err)
					}
					return
				}
				if err != nil || len(files) != 0 {
					t.Fatalf("want silent skip, got files=%v err=%v", files, err)
				}
			})
			t.Run("v1.4", func(t *testing.T) {
				compattest.Set(t, false)
				dir := t.TempDir()
				installPlugin(t, dir, "p", tc.manifest, "", tc.extra...)
				var files []string
				var err error
				out := captureStderr(t, func() {
					files, err = discoverPluginComposeFilesFx(writeEffects{}, newDiskSink(dir), dir, dir)
				})
				if err != nil || len(files) != 0 {
					t.Fatalf("v1.4 must skip without failing, got files=%v err=%v", files, err)
				}
				if got := strings.Contains(out, "no docker-compose.plugin.yml"); got != tc.compose {
					t.Fatalf("warning present = %v, want %v (stderr %q)", got, tc.compose, out)
				}
			})
		})
	}
}

// TestFragmentNetworkNormalize: v1.5 attaches a missing network and writes
// ${DOCKER_NETWORK:-x} as ${DOCKER_NETWORK}; the result is idempotent; v1.4
// leaves every byte alone.
func TestFragmentNetworkNormalize(t *testing.T) {
	t.Run("v1.5 attaches missing network", func(t *testing.T) {
		compattest.Set(t, true)
		out, err := normalizeComposeNetworks([]byte(fragNoNetworks), "activity-feed")
		if err != nil {
			t.Fatal(err)
		}
		want := "services:\n  activity-feed:\n    networks:\n      - ${DOCKER_NETWORK}\n    build: ."
		if !strings.HasPrefix(string(out), want) {
			t.Fatalf("network not attached first in the service:\n%s", out)
		}
		again, _ := normalizeComposeNetworks(out, "activity-feed")
		if string(again) != string(out) {
			t.Fatalf("not idempotent:\n%s\n---\n%s", out, again)
		}
	})
	t.Run("v1.5 rewrites the default form", func(t *testing.T) {
		compattest.Set(t, true)
		out, err := normalizeComposeNetworks([]byte(fragDefaultNetwork), "nself-audit")
		if err != nil {
			t.Fatal(err)
		}
		s := string(out)
		if strings.Contains(s, "nself_network") || !strings.Contains(s, "    networks:\n      - ${DOCKER_NETWORK}\n") {
			t.Fatalf("default form not rewritten:\n%s", s)
		}
		if want := strings.Replace(fragDefaultNetwork, "${DOCKER_NETWORK:-nself_network}", "${DOCKER_NETWORK}", 1); s != want {
			t.Fatalf("only that value may change:\n%s", s)
		}
	})
	t.Run("v1.5 leaves the project external network and network_mode alone", func(t *testing.T) {
		compattest.Set(t, true)
		out, err := normalizeComposeNetworks([]byte(fragProjectExternal), "browser")
		if err != nil || string(out) != fragProjectExternal {
			t.Fatalf("project network must pass untouched, err=%v\n%s", err, out)
		}
		host := "services:\n  h:\n    image: x\n    network_mode: host\n"
		if out, _ := normalizeComposeNetworks([]byte(host), "h"); string(out) != host {
			t.Fatalf("network_mode service must not get networks:\n%s", out)
		}
	})
	t.Run("v1.4 changes nothing", func(t *testing.T) {
		compattest.Set(t, false)
		for _, f := range []string{fragNoNetworks, fragDefaultNetwork, fragProjectExternal} {
			out, err := normalizeComposeNetworks([]byte(f), "x")
			if err != nil || string(out) != f {
				t.Fatalf("v1.4 must not rewrite, err=%v\n%s", err, out)
			}
		}
	})
	t.Run("through discovery, written back", func(t *testing.T) {
		compattest.Set(t, true)
		dir := t.TempDir()
		installPlugin(t, dir, "nself-audit", `{"name":"nself-audit","port":3843}`, fragDefaultNetwork)
		installPlugin(t, dir, "activity-feed", `{"name":"activity-feed","port":3209}`, fragNoNetworks)
		if _, err := discoverPluginComposeFilesFx(writeEffects{}, newDiskSink(dir), dir, dir); err != nil {
			t.Fatal(err)
		}
		if got := readFragment(t, dir, "nself-audit"); strings.Contains(got, ":-nself_network") {
			t.Fatalf("fragment on disk still has the default form:\n%s", got)
		}
		if got := readFragment(t, dir, "activity-feed"); !strings.Contains(got, "networks:\n      - ${DOCKER_NETWORK}") {
			t.Fatalf("fragment on disk has no network:\n%s", got)
		}
	})
}

// TestFragmentNetworkForeignE129: an external network other than the project
// network is E129 naming plugin and network (v1.5), a warning (v1.4).
func TestFragmentNetworkForeignE129(t *testing.T) {
	t.Run("v1.5", func(t *testing.T) {
		compattest.Set(t, true)
		_, err := normalizeComposeNetworks([]byte(fragForeign), "spy")
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E129" || !strings.Contains(ce.What, `"spy"`) || !strings.Contains(ce.What, "traefik_proxy") {
			t.Fatalf("want E129 naming plugin and network, got %v", err)
		}
		dir := t.TempDir()
		installPlugin(t, dir, "spy", `{"name":"spy"}`, fragForeign)
		if _, err := discoverPluginComposeFilesFx(writeEffects{}, newDiskSink(dir), dir, dir); !errors.As(err, &ce) || ce.Code != "E129" {
			t.Fatalf("discovery must fail the build with E129, got %v", err)
		}
		if got := readFragment(t, dir, "spy"); got != fragForeign {
			t.Fatalf("a refused fragment must not be rewritten:\n%s", got)
		}
	})
	t.Run("v1.4", func(t *testing.T) {
		compattest.Set(t, false)
		var out []byte
		var err error
		stderr := captureStderr(t, func() { out, err = normalizeComposeNetworks([]byte(fragForeign), "spy") })
		if err != nil || string(out) != fragForeign {
			t.Fatalf("v1.4 must only warn, err=%v", err)
		}
		if !strings.Contains(stderr, "spy") || !strings.Contains(stderr, "traefik_proxy") {
			t.Fatalf("warning should name plugin and network, got %q", stderr)
		}
	})
	t.Run("name forms", func(t *testing.T) {
		compattest.Set(t, true)
		for frag, foreign := range map[string]bool{
			"networks:\n  a:\n    external: true\n":                                      true, // literal key is the name
			"networks:\n  a:\n    external:\n      name: other\n":                        true,
			"networks:\n  a:\n    external: true\n    name: ${DOCKER_NETWORK:-x}\n":      false,
			"networks:\n  a:\n    external: true\n    name: ${PROJECT_NAME}_network\n":   false,
			"networks:\n  a:\n    external: false\n    name: other\n":                    false,
			"networks:\n  a:\n    driver: bridge\n":                                      false,
			"networks:\n  a:\n    external: true\n    name: ${COMPOSE_PROJECT_NAME}_x\n": true,
		} {
			_, err := normalizeComposeNetworks([]byte("services:\n  s:\n    image: x\n"+frag), "s")
			if (err != nil) != foreign {
				t.Errorf("%q: foreign=%v but err=%v", frag, foreign, err)
			}
		}
	})
}

// TestPluginsCoreEnvPort: v1.5 fragments receive PORT equal to SERVICE_PORT;
// a PORT the fragment authored wins; port 0 adds none; v1.4 is unchanged.
func TestPluginsCoreEnvPort(t *testing.T) {
	frag := "services:\n  svc:\n    image: x\n    environment:\n      DATABASE_URL: ${DATABASE_URL}\n    networks:\n      - ${DOCKER_NETWORK}\n"
	run := func(t *testing.T, port int, in string) string {
		pluginDir := t.TempDir()
		writePluginManifestJSON(t, pluginDir, "svc", port)
		return string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "svc"))
	}
	t.Run("v1.5", func(t *testing.T) {
		compattest.Set(t, true)
		out := run(t, 3843, frag)
		if !strings.Contains(out, `SERVICE_PORT: "3843"`) || !strings.Contains(out, `PORT: "3843"`) {
			t.Fatalf("want SERVICE_PORT and PORT 3843:\n%s", out)
		}
		if n := strings.Count(out, "\n      PORT:"); n != 1 {
			t.Fatalf("PORT must appear once, got %d:\n%s", n, out)
		}
		authored := strings.Replace(frag, "DATABASE_URL: ${DATABASE_URL}", "DATABASE_URL: ${DATABASE_URL}\n      PORT: ${PLUGIN_PORT:-9}", 1)
		out = run(t, 3843, authored)
		if !strings.Contains(out, "PORT: ${PLUGIN_PORT:-9}") || strings.Contains(out, "\n      PORT: \"3843\"") {
			t.Fatalf("authored PORT must win:\n%s", out)
		}
		if out := run(t, 0, frag); strings.Contains(out, "\n      PORT:") {
			t.Fatalf("no PORT without a port:\n%s", out)
		}
		again := run(t, 3843, run(t, 3843, frag))
		if again != run(t, 3843, frag) {
			t.Fatal("injection is not idempotent")
		}
	})
	t.Run("v1.4", func(t *testing.T) {
		compattest.Set(t, false)
		if out := run(t, 3843, frag); strings.Contains(out, "\n      PORT:") {
			t.Fatalf("v1.4 must not add PORT:\n%s", out)
		}
	})
}

// TestE130Registered: E130 is the fragment-policy code P7-DEPL-21 raises.
func TestE130Registered(t *testing.T) {
	e, ok := errs.Registry["E130"]
	if !ok {
		t.Fatal("E130 is not registered")
	}
	if e.Summary != "plugin compose fragment violates policy (ADR 0027)" || e.Exit != 1 || e.Category != "plugin" {
		t.Fatalf("E130 = %+v", e)
	}
	for _, c := range []string{"E128", "E129", "E131"} {
		if _, ok := errs.Registry[c]; !ok {
			t.Errorf("%s is not registered", c)
		}
	}
}
