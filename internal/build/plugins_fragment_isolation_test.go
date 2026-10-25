package build

// Purpose: D-0490 tests for the fragment isolation rules: include, extends of
// another file, build.network, an undeclared network and a core service name
// are E129 in v1.5 (a warning in v1.4), and the first-party fragment shapes
// stay clean.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestFragmentIsolationRefusals(t *testing.T) {
	cases := []struct {
		name, frag, want string
	}{
		{"include", "include:\n  - other.yml\nservices:\n  w:\n    image: x\n    networks: [\"${DOCKER_NETWORK}\"]\n", "top-level include"},
		{"extends file", "services:\n  w:\n    extends:\n      file: ../other.yml\n      service: base\n    networks: [\"${DOCKER_NETWORK}\"]\n", "w: extends another file"},
		{"extends short form", "services:\n  w:\n    extends: base\n    networks: [\"${DOCKER_NETWORK}\"]\n", "w: extends another file"},
		{"extends via merge key", "x-base: &b\n  extends:\n    file: ../other.yml\n    service: base\nservices:\n  w:\n    <<: *b\n    image: x\n    networks: [\"${DOCKER_NETWORK}\"]\n", "w: extends another file"},
		{"build network host", "services:\n  w:\n    build:\n      context: .\n      network: host\n    networks: [\"${DOCKER_NETWORK}\"]\n", "w: build.network host"},
		{"build network by name", "services:\n  w:\n    build:\n      context: .\n      network: HOST\n    networks: [\"${DOCKER_NETWORK}\"]\n", "w: build.network host"},
		{"undeclared sibling network", "services:\n  w:\n    image: x\n    networks:\n      - ${DOCKER_NETWORK}\n      - ${COMPOSE_PROJECT_NAME}_b_private\n", "network ${COMPOSE_PROJECT_NAME}_b_private is not declared"},
		{"undeclared network, mapping form", "services:\n  w:\n    image: x\n    networks:\n      b_private: {}\n", "network b_private is not declared"},
		{"core service hasura", "services:\n  hasura:\n    image: x\n    networks: [\"${DOCKER_NETWORK}\"]\n", "hasura: defines the core service hasura"},
		{"core service postgres", "services:\n  postgres:\n    image: x\n    networks: [\"${DOCKER_NETWORK}\"]\n", "postgres: defines the core service postgres"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) { wantE129(t, c.frag, c.want) })
		})
	}
}

func TestFragmentIsolationAllowed(t *testing.T) {
	cases := map[string]string{
		"project network only":    "services:\n  w:\n    image: x\n    networks: [\"${DOCKER_NETWORK}\"]\n",
		"project network default": "services:\n  w:\n    image: x\n    networks: [\"${DOCKER_NETWORK:-x}\", default]\n",
		"declared own network":    "services:\n  w:\n    image: x\n    networks: [\"${DOCKER_NETWORK}\", private]\nnetworks:\n  private:\n    name: \"${COMPOSE_PROJECT_NAME}_w_private\"\n",
		"same-file extends":       "services:\n  base:\n    image: x\n    networks: [\"${DOCKER_NETWORK}\"]\n  w:\n    extends:\n      service: base\n",
		"build network none":      "services:\n  w:\n    build:\n      context: .\n      network: none\n    networks: [\"${DOCKER_NETWORK}\"]\n",
		"build without network":   "services:\n  w:\n    build: .\n    networks: [\"${DOCKER_NETWORK}\"]\n",
		"optional service name":   "services:\n  redis:\n    image: redis:7\n    networks: [\"${DOCKER_NETWORK}\"]\n",
	}
	for name, frag := range cases {
		frag := frag
		t.Run(name, func(t *testing.T) {
			r, err := resolveFragment([]byte(frag))
			if err != nil {
				t.Fatal(err)
			}
			if v := r.isolationViolations(); len(v) != 0 {
				t.Errorf("violations %q on a fragment that stays inside its stack", v)
			}
		})
	}
}

// TestFragmentIsolationFirstPartyFixtures: the recorded first-party fragments
// break none of the rules.
func TestFragmentIsolationFirstPartyFixtures(t *testing.T) {
	for _, glob := range []string{"testdata/plugin-compose-fixtures/*.yml", "testdata/legacy_networks/*.yml"} {
		files, err := filepath.Glob(glob)
		if err != nil || len(files) == 0 {
			t.Fatalf("no fixtures for %s (err=%v)", glob, err)
		}
		for _, p := range files {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			r, err := resolveFragment(b)
			if err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			if v := r.isolationViolations(); len(v) != 0 {
				t.Errorf("%s: %q", p, v)
			}
		}
	}
}
