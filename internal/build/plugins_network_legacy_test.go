package build

// Purpose: P7-PLUG-17 tests for the legacy literal external network
// `nself_network`: it is the project network (no E129 in v1.5, no warning in
// v1.4), v1.5 names its definition ${DOCKER_NETWORK}, v1.4 rewrites nothing.

import (
	"errors"
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

// networkName returns networks.<key>.name from the YAML in s.
func networkName(t *testing.T, s, key string) string {
	t.Helper()
	var doc struct {
		Networks map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"networks"`
	}
	if err := yaml.Unmarshal([]byte(s), &doc); err != nil {
		t.Fatalf("output is not valid YAML: %v\n%s", err, s)
	}
	return doc.Networks[key].Name
}

// TestLegacyNselfNetworkIsProjectNetwork: block and flow style, both modes.
func TestLegacyNselfNetworkIsProjectNetwork(t *testing.T) {
	for name, frag := range map[string]string{"block": fragLegacyBlock, "flow": fragLegacyFlow} {
		frag := frag
		t.Run(name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				resetWarnOnce()
				var out []byte
				var err error
				stderr := captureStderr(t, func() { out, err = normalizeComposeNetworks([]byte(frag), "access-controls") })
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
				if got := networkName(t, string(out), "nself_network"); got != "${DOCKER_NETWORK}" {
					t.Fatalf("v1.5 name = %q, want ${DOCKER_NETWORK}:\n%s", got, out)
				}
				if name == "block" && !strings.Contains(string(out), "  nself_network:\n    name: ${DOCKER_NETWORK}\n    external: true\n") {
					t.Fatalf("block style splice:\n%s", out)
				}
				if name == "flow" && !strings.Contains(string(out), `nself_network: {external: true, name: "${DOCKER_NETWORK}"}`) {
					t.Fatalf("flow style splice:\n%s", out)
				}
				again, err := normalizeComposeNetworks(out, "access-controls")
				if err != nil || string(again) != string(out) {
					t.Fatalf("v1.5 rewrite must be idempotent (err %v):\n%s", err, again)
				}
			})
		})
	}
}

// TestLegacyNselfNetworkNameForms: a literal name: is the project network too
// and is replaced in v1.5; a variable-based name is left alone; a real foreign
// network next to the legacy one is still E129 in v1.5.
func TestLegacyNselfNetworkNameForms(t *testing.T) {
	compattest.Set(t, true)
	head := "services:\n  s:\n    image: x\nnetworks:\n"
	for _, tc := range []struct{ name, in, want string }{
		{"literal name", head + "  default:\n    external: true\n    name: nself_network\n", "    name: ${DOCKER_NETWORK}\n"},
		{"quoted literal name", head + "  default:\n    external: true\n    name: \"nself_network\"\n", "    name: \"${DOCKER_NETWORK}\"\n"},
		{"flow literal name", head + "  default: {external: true, name: nself_network}\n", `default: {external: true, name: "${DOCKER_NETWORK}"}`},
		{"external name form", head + "  default:\n    external:\n      name: nself_network\n", "      name: ${DOCKER_NETWORK}\n"},
		{"variable name untouched", head + "  nself_network:\n    external: true\n    name: ${COMPOSE_PROJECT_NAME}_network\n", ""},
	} {
		out, err := normalizeComposeNetworks([]byte(tc.in), "s")
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if tc.want == "" {
			// (the service still gets its own network attached; only the def is checked)
			if !strings.HasSuffix(string(out), tc.in[strings.Index(tc.in, "\nnetworks:"):]) {
				t.Errorf("%s: network def must be left alone:\n%s", tc.name, out)
			}
			continue
		}
		if !strings.Contains(string(out), tc.want) || strings.Contains(string(out), "name: nself_network") {
			t.Errorf("%s: want %q in:\n%s", tc.name, tc.want, out)
		}
		if err := assertValidComposeYAML(string(out)); err != nil {
			t.Errorf("%s: invalid YAML: %v\n%s", tc.name, err, out)
		}
	}
	_, err := normalizeComposeNetworks([]byte(fragLegacyBlock+"  traefik_proxy:\n    external: true\n"), "s")
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E129" || !strings.Contains(ce.What, "traefik_proxy") || strings.Contains(ce.What, "nself_network") {
		t.Fatalf("a real foreign network must still be E129 naming only it, got %v", err)
	}
}
