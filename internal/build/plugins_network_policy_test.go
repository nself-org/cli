package build

// Purpose: P7-PLUG-17 round 5 tests for the network policy: external written
// as a string, a named non-external network, the legacy literal set on real
// first-party fragment shapes, a dedented comment inside a dropped definition,
// and a fragment that does not decode. Fixtures in testdata/legacy_networks
// are copies of the first-party fragments (no reads of the plugins repo).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

// wantE129 asserts v1.5 raises E129 mentioning each of subs; v1.4 does not fail.
func wantE129(t *testing.T, frag string, subs ...string) {
	t.Helper()
	resetWarnOnce()
	var out []byte
	var err error
	stderr := captureStderr(t, func() { out, err = normalizeComposeNetworks([]byte(frag), "spy") })
	if compat.V15() {
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E129" {
			t.Fatalf("v1.5: want E129, got %v", err)
		}
		for _, s := range subs {
			if !strings.Contains(ce.What, s) {
				t.Fatalf("E129 should mention %q: %s", s, ce.What)
			}
		}
		return
	}
	if err != nil || string(out) != frag {
		t.Fatalf("v1.4 must not fail or rewrite, err %v", err)
	}
	_ = stderr
}

// TestNetworkPolicyExternalStrings (NM1): external as a string is external
// (fail closed), except the false spellings.
func TestNetworkPolicyExternalStrings(t *testing.T) {
	for _, ext := range []string{`"true"`, `yes`, `on`, `"${EXT:-true}"`, `True`, `1`} {
		ext := ext
		t.Run(ext, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				// The key is the network's real name, so only isExternal can catch it.
				frag := "services:\n  w:\n    image: x\n    networks: [traefik_proxy]\nnetworks:\n  traefik_proxy:\n    external: " + ext + "\n"
				wantE129(t, frag, "traefik_proxy")
			})
		})
	}
	compattest.Set(t, true)
	for _, ext := range []string{"false", "False", `"no"`, "off", "n", "null"} {
		frag := "services:\n  w:\n    image: x\n    networks: [proxy]\nnetworks:\n  proxy:\n    external: " + ext + "\n"
		if _, err := normalizeComposeNetworks([]byte(frag), "w"); err != nil {
			t.Errorf("external: %s is not external, got %v", ext, err)
		}
	}
}

// TestNetworkPolicyNamedNonExternal (NM2): a non-external network whose
// explicit name is not project-scoped joins an existing network of that name.
func TestNetworkPolicyNamedNonExternal(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		wantE129(t, "services:\n  w:\n    image: x\n    networks: [proxy]\nnetworks:\n  proxy:\n    name: traefik_proxy\n", "traefik_proxy")
		wantE129(t, "services:\n  w:\n    image: x\n    networks: [p]\nnetworks:\n  p:\n    external: false\n    name: other\n", "other")
	})
	compattest.Both(t, func(t *testing.T) {
		for _, name := range []string{"${COMPOSE_PROJECT_NAME:-nself}_forgejo_runner_net", "${PROJECT_NAME}_x", "${DOCKER_NETWORK}", "${COMPOSE_PROJECT_NAME}_network"} {
			frag := "services:\n  w:\n    image: x\n    networks: [p]\nnetworks:\n  p:\n    name: " + name + "\n"
			if _, err := normalizeComposeNetworks([]byte(frag), "w"); err != nil {
				t.Errorf("%s is project-scoped, got %v", name, err)
			}
		}
	})
}

// TestLegacyDropAcrossDedentedComment (SF1): a column-0 comment inside the
// dropped definition must not leave its children under the previous key.
func TestLegacyDropAcrossDedentedComment(t *testing.T) {
	compattest.Set(t, true)
	frag := "services:\n  web:\n    image: x\n    networks: [nself_network]\nnetworks:\n  other:\n    driver: bridge\n  nself_network:\n    external: true\n# keep\n    labels:\n      evil: \"1\"\n  last:\n    driver: bridge\n# after\n"
	out, err := normalizeComposeNetworks([]byte(frag), "web")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "evil") || strings.Contains(string(out), "labels") {
		t.Fatalf("the whole definition must go:\n%s", out)
	}
	if !strings.Contains(string(out), "# after") || !strings.Contains(string(out), "  last:\n    driver: bridge\n") {
		t.Fatalf("what follows the definition must stay:\n%s", out)
	}
	svc, nets := parsed(t, string(out))
	if len(nets) != 2 || strings.Join(svc["web"], ",") != "${DOCKER_NETWORK}" {
		t.Fatalf("svc %v nets %v:\n%s", svc, nets, out)
	}
}

// TestLegacyNetworkRealFragments (round 5): the first-party fragments that use
// nself_network, nself-network or nself fold in v1.5 (no E129, definition and
// references gone, valid YAML) and are untouched in v1.4; forgejo keeps its
// own project-scoped runner network.
func TestLegacyNetworkRealFragments(t *testing.T) {
	files, err := filepath.Glob("testdata/legacy_networks/*.yml")
	if err != nil || len(files) < 9 {
		t.Fatalf("fixtures missing: %v %v", files, err)
	}
	for _, f := range files {
		f := f
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			compattest.Both(t, func(t *testing.T) {
				resetWarnOnce()
				var out []byte
				var nerr error
				stderr := captureStderr(t, func() { out, nerr = normalizeComposeNetworks(raw, "p") })
				if nerr != nil {
					t.Fatalf("no E129 for a first-party fragment: %v", nerr)
				}
				if !compat.V15() {
					if string(out) != string(raw) || strings.Contains(stderr, "other than the project network") {
						t.Fatalf("v1.4: unchanged and no network warning, stderr %q", stderr)
					}
					return
				}
				res, derr := resolveFragment(out)
				if derr != nil {
					t.Fatalf("output does not decode: %v\n%s", derr, out)
				}
				if len(res.legacyKeys()) != 0 {
					t.Fatalf("legacy definition left:\n%s", out)
				}
				for name, s := range res.Services {
					if s.refersTo(map[string]bool{"nself_network": true, "nself-network": true, "nself": true}) {
						t.Fatalf("service %s still lists a legacy network:\n%s", name, out)
					}
				}
				if filepath.Base(f) == "forgejo.yml" && res.Networks["forgejo_runner_net"].Name == "" {
					t.Fatalf("forgejo's own runner network must survive:\n%s", out)
				}
			})
		})
	}
}

// TestNetworkPolicyUndecodableFailsClosed (round 5): parses as nodes, not as
// values: E129 in v1.5 (policy could not be checked), unchanged in v1.4.
func TestNetworkPolicyUndecodableFailsClosed(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		wantE129(t, "services:\n  w:\n    image: x\nnetworks:\n  p:\n    name: [a, b]\n", "could not be checked")
	})
}

// TestMergedServiceUnderServices: a service supplied by a merge key under
// services: cannot be given a networks: key, so v1.5 refuses it.
func TestMergedServiceUnderServices(t *testing.T) {
	frag := "x-s: &s\n  web:\n    image: x\nservices:\n  <<: *s\n  db:\n    image: y\n    networks: [n]\nnetworks:\n  n:\n    driver: bridge\n"
	compattest.Both(t, func(t *testing.T) { wantE129(t, frag, `"web"`) })
}
