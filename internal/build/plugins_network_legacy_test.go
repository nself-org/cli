package build

// Purpose: P7-PLUG-17 tests for the legacy literal external network
// `nself_network`: it is the project network (no E129 for it in either mode),
// v1.5 folds it into ${DOCKER_NETWORK} (definition deleted, service references
// rewritten), v1.4 rewrites nothing, and a shape the build cannot rewrite
// line-locally is E129 in v1.5.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

// fragLegacyBlock is the access-controls shape.
const fragLegacyBlock = `services:
  access-controls:
    image: redis:7.4
    networks: [nself_network]
networks:
  nself_network:
    external: true
`

const fragLegacyFlow = `services:
  access-controls:
    image: redis:7.4
    networks: [nself_network]
networks:
  nself_network: {external: true}
`

// fragForgejo is the forgejo shape: comments inside networks:, a second
// non-legacy network that must survive.
const fragForgejo = `services:
  forgejo:
    image: redis:7.4
    networks:
      - nself_network
      - backend
networks:
  # the project network, created by nself
  nself_network:
    # external so compose reuses it
    external: true
  backend:
    driver: bridge
# trailing comment
`

// parsed decodes out as YAML and returns the service networks and top-level
// network names.
func parsed(t *testing.T, out string) (svc map[string][]string, nets map[string]any) {
	t.Helper()
	var doc struct {
		Services map[string]struct {
			Networks []string `yaml:"networks"`
		} `yaml:"services"`
		Networks map[string]any `yaml:"networks"`
	}
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid YAML: %v\n%s", err, out)
	}
	svc = map[string][]string{}
	for k, v := range doc.Services {
		svc[k] = v.Networks
	}
	return svc, doc.Networks
}

// TestLegacyNselfNetworkFolds: both modes for the access-controls (block and
// flow) and forgejo shapes.
func TestLegacyNselfNetworkFolds(t *testing.T) {
	for name, frag := range map[string]string{"block": fragLegacyBlock, "flow": fragLegacyFlow, "forgejo": fragForgejo} {
		name, frag := name, frag
		t.Run(name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				resetWarnOnce()
				var out []byte
				var err error
				stderr := captureStderr(t, func() { out, err = normalizeComposeNetworks([]byte(frag), "x") })
				if err != nil {
					t.Fatalf("legacy nself_network must not be refused: %v", err)
				}
				if strings.Contains(stderr, "nself_network") {
					t.Fatalf("legacy nself_network must not warn, got %q", stderr)
				}
				if !compat.V15() {
					if string(out) != frag {
						t.Fatalf("v1.4 must rewrite nothing:\n%s", out)
					}
					return
				}
				svc, nets := parsed(t, string(out))
				if strings.Contains(string(out), "nself_network") {
					t.Fatalf("output still names nself_network:\n%s", out)
				}
				if name == "forgejo" {
					if len(nets) != 1 || nets["backend"] == nil || strings.Join(svc["forgejo"], ",") != "${DOCKER_NETWORK},backend" {
						t.Fatalf("forgejo: backend must survive, refs rewritten (svc %v nets %v):\n%s", svc, nets, out)
					}
					if strings.Contains(string(out), "external so") {
						t.Fatalf("comments inside the dropped def must go:\n%s", out)
					}
					if !strings.Contains(string(out), "# trailing comment") {
						t.Fatalf("a comment after the def must stay:\n%s", out)
					}
				} else {
					if nets != nil {
						t.Fatalf("the only entry: the whole networks: key must go, got %v:\n%s", nets, out)
					}
					if got := strings.Join(svc["access-controls"], ","); got != "${DOCKER_NETWORK}" {
						t.Fatalf("service networks = %q, want ${DOCKER_NETWORK} (no duplicate):\n%s", got, out)
					}
				}
				again, err := normalizeComposeNetworks(out, "x")
				if err != nil || string(again) != string(out) {
					t.Fatalf("v1.5 rewrite must be idempotent (err %v):\n%s", err, again)
				}
			})
		})
	}
}

// TestLegacyNselfNetworkNameForms: a literal name: (under any key, quoted, or
// inside external:) is the legacy network too; a variable-based name is not.
func TestLegacyNselfNetworkNameForms(t *testing.T) {
	compattest.Set(t, true)
	svc := "services:\n  s:\n    image: x\n    networks:\n      - default\n"
	for _, tc := range []struct {
		name, nets string
		legacy     bool
	}{
		{"literal name", "  default:\n    external: true\n    name: nself_network\n", true},
		{"quoted literal name", "  default:\n    external: true\n    name: \"nself_network\"\n", true},
		{"flow literal name", "  default: {external: true, name: nself_network}\n", true},
		{"external name form", "  default:\n    external:\n      name: nself_network\n", true},
		{"variable name", "  default:\n    external: true\n    name: ${COMPOSE_PROJECT_NAME}_network\n", false},
	} {
		out, err := normalizeComposeNetworks([]byte(svc+"networks:\n"+tc.nets), "s")
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		_, nets := parsed(t, string(out))
		if tc.legacy && (nets != nil || !strings.Contains(string(out), "- ${DOCKER_NETWORK}")) {
			t.Errorf("%s: want the def dropped and the ref rewritten:\n%s", tc.name, out)
		}
		if !tc.legacy && nets == nil {
			t.Errorf("%s: a variable-named network must stay:\n%s", tc.name, out)
		}
	}
}

// TestLegacyNselfNetworkE129: v1.5 refuses a legacy network it cannot rewrite
// line-locally; v1.4 leaves it alone; a real foreign network still E129.
func TestLegacyNselfNetworkE129(t *testing.T) {
	cases := map[string]string{
		"multi-line flow def":                "services:\n  s:\n    image: x\n    networks: [nself_network]\nnetworks:\n  other:\n    driver: bridge\n  nself_network: {\n    external: true\n  }\n",
		"quoted brace":                       "services:\n  s:\n    image: x\n    networks: [nself_network]\nnetworks:\n  other:\n    driver: bridge\n  nself_network: {external: true, labels: {a: \"}}\"}\n    }\n",
		"mapping-form ref":                   "services:\n  s:\n    image: x\n    networks:\n      nself_network:\n        aliases: [a]\nnetworks:\n  nself_network:\n    external: true\n",
		"ref via merge key":                  "x-base: &base\n  image: x\n  networks: [nself_network]\nservices:\n  s:\n    <<: *base\nnetworks:\n  nself_network:\n    external: true\n",
		"networks via merge key":             "x-nets: &nets {nself_network: {external: true}}\nservices:\n  s:\n    image: x\n    networks: [nself_network]\nnetworks:\n  <<: *nets\n",
		"service already on project network": "services:\n  s:\n    image: x\n    networks: [nself_network, \"${DOCKER_NETWORK}\"]\nnetworks:\n  nself_network:\n    external: true\n",
	}
	for name, frag := range cases {
		name, frag := name, frag
		t.Run(name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				out, err := normalizeComposeNetworks([]byte(frag), "s")
				var ce *errs.CLIError
				if compat.V15() {
					if !errors.As(err, &ce) || ce.Code != "E129" || !strings.Contains(ce.What, `"s"`) || !strings.Contains(ce.What, "legacy external project network") {
						t.Fatalf("want E129 naming plugin and nself_network, got %v", err)
					}
					return
				}
				if err != nil || string(out) != frag {
					t.Fatalf("v1.4 must not fail or rewrite, err %v", err)
				}
			})
		})
	}
	t.Run("foreign next to legacy", func(t *testing.T) {
		compattest.Set(t, true)
		_, err := normalizeComposeNetworks([]byte(fragLegacyBlock+"  traefik_proxy:\n    external: true\n"), "s")
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E129" || !strings.Contains(ce.What, "traefik_proxy") || strings.Contains(ce.What, "nself_network") {
			t.Fatalf("a real foreign network must still be E129 naming only it, got %v", err)
		}
	})
}

// TestForeignNetworkResolvedView: an alias or merge key cannot hide a foreign
// external network (E129 in v1.5, a warning in v1.4), and a service that gets
// networks through a merge key is not given a second networks: key.
func TestForeignNetworkResolvedView(t *testing.T) {
	cases := map[string]string{
		"alias def":        "x-ext: &ext {external: true, name: traefik_proxy}\nservices:\n  w:\n    image: x\n    networks: [proxy]\nnetworks:\n  proxy: *ext\n",
		"merge def":        "x-ext: &ext {external: true, name: traefik_proxy}\nservices:\n  w:\n    image: x\n    networks: [proxy]\nnetworks:\n  proxy:\n    <<: *ext\n",
		"capitalised bool": "services:\n  w:\n    image: x\n    networks: [proxy]\nnetworks:\n  proxy:\n    external: True\n    name: traefik_proxy\n",
	}
	for name, frag := range cases {
		name, frag := name, frag
		t.Run(name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				resetWarnOnce()
				var out []byte
				var err error
				stderr := captureStderr(t, func() { out, err = normalizeComposeNetworks([]byte(frag), "spy") })
				var ce *errs.CLIError
				if compat.V15() {
					if !errors.As(err, &ce) || ce.Code != "E129" || !strings.Contains(ce.What, "traefik_proxy") {
						t.Fatalf("want E129 naming traefik_proxy, got %v", err)
					}
					return
				}
				if err != nil || string(out) != frag || !strings.Contains(stderr, "traefik_proxy") {
					t.Fatalf("v1.4 must warn and rewrite nothing, err %v stderr %q", err, stderr)
				}
			})
		})
	}
	t.Run("merge-key service keeps its networks", func(t *testing.T) {
		compattest.Set(t, true)
		frag := "x-base: &base\n  image: nginx\n  networks: [backend]\nservices:\n  web:\n    <<: *base\n  mode:\n    image: x\n    network_mode: host\n  plain:\n    image: x\nnetworks:\n  backend:\n    driver: bridge\n"
		out, err := normalizeComposeNetworks([]byte(frag), "w")
		if err != nil {
			t.Fatal(err)
		}
		svc, _ := parsed(t, string(out))
		if strings.Contains(string(out), "web:\n    networks") || strings.Contains(string(out), "<<: *base\n    networks") {
			t.Fatalf("web gets its networks through the merge key; nothing may be inserted:\n%s", out)
		}
		if got := strings.Join(svc["plain"], ","); got != "${DOCKER_NETWORK}" {
			t.Fatalf("a plain service still gets the project network, got %q:\n%s", got, out)
		}
		if len(svc["mode"]) != 0 {
			t.Fatalf("network_mode service must be left alone:\n%s", out)
		}
	})
}

// composeCmd returns the compose command on this machine, or nil.
func composeCmd() []string {
	if exec.Command("docker", "compose", "version").Run() == nil {
		return []string{"docker", "compose"}
	}
	if p, err := exec.LookPath("docker-compose"); err == nil {
		return []string{p}
	}
	return nil
}

// TestLegacyNselfNetworkComposeConfig: the folded fragment merges with a main
// compose file into one valid project with a single network and no external
// definition left (skipped when compose is not installed).
func TestLegacyNselfNetworkComposeConfig(t *testing.T) {
	cc := composeCmd()
	if cc == nil {
		t.Skip("docker compose not installed")
	}
	compattest.Set(t, true)
	dir := t.TempDir()
	main := "services:\n  main:\n    image: redis:7.4\n    networks:\n      - ${DOCKER_NETWORK}\nnetworks:\n  plug17_network:\n    driver: bridge\n    name: plug17_network\n"
	if err := os.WriteFile(filepath.Join(dir, "main.yml"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, frag := range map[string]string{"access-controls": fragLegacyBlock, "forgejo": fragForgejo} {
		out, err := normalizeComposeNetworks([]byte(frag), name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".yml"), out, 0o644); err != nil {
			t.Fatal(err)
		}
		args := append(append([]string{}, cc[1:]...), "-p", "plug17", "-f", "main.yml", "-f", name+".yml", "config")
		cmd := exec.Command(cc[0], args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "DOCKER_NETWORK=plug17_network")
		got, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: compose config failed: %v\n%s\n--- fragment:\n%s", name, err, got, out)
		}
		if strings.Contains(string(got), "nself_network") || strings.Contains(string(got), "external") {
			t.Fatalf("%s: merged config still has the legacy or an external network:\n%s", name, got)
		}
	}
}
