package compose

// Purpose: pin the compose SERVICE name for the admin UI against the three
// near-identical names for the same thing (service "nself-admin", image pin
// key "admin", container "<project>_admin").
//
// Inputs:  a generator config with Admin.Enabled.
// Outputs: assertions on the generated service map.
// Constraints: regression guard for the E2E golden path step 11 failure —
// `nself admin start` passed "admin" to `docker compose up --no-deps`, which
// docker rejects with "no such service: admin". The stack came up healthy and
// only this one call was wrong, so the whole release smoke failed on a
// one-word mismatch.

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/config"
)

// generatedServices marshals the compose file the generator actually emits and
// returns its service keys. Asserting on the serialized YAML rather than the
// in-memory struct means the test checks exactly what docker will read.
func generatedServices(t *testing.T, g *Generator) map[string]struct{} {
	t.Helper()

	raw, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc struct {
		Services map[string]yaml.Node `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal generated compose: %v", err)
	}

	names := make(map[string]struct{}, len(doc.Services))
	for n := range doc.Services {
		names[n] = struct{}{}
	}
	return names
}

// keys lists a service set for failure messages.
func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// adminEnabledGenerator returns a generator whose config has the admin UI on.
func adminEnabledGenerator() *Generator {
	cfg := minimalCfg()
	cfg.Admin = config.AdminConfig{Enabled: true, Port: 3021, Version: "latest"}
	return NewGenerator(cfg)
}

func TestAdminServiceName_IsNotTheImageKey(t *testing.T) {
	// "admin" is the image pin key and the container suffix. If the service
	// name ever collapses to it, every `docker compose ... nself-admin` call
	// breaks instead — so assert they stay distinct deliberately.
	if AdminServiceName == "admin" {
		t.Fatal("AdminServiceName must not be \"admin\": that is the image pin key, not the compose service")
	}
	if AdminServiceName != "nself-admin" {
		t.Fatalf("AdminServiceName = %q, want \"nself-admin\"", AdminServiceName)
	}
}

func TestGeneratedComposeUsesAdminServiceName(t *testing.T) {
	svcs := generatedServices(t, adminEnabledGenerator())

	if _, ok := svcs[AdminServiceName]; !ok {
		t.Fatalf("generated compose has no %q service; services present: %v",
			AdminServiceName, keys(svcs))
	}

	// The bare image key must NOT be a service. This is the assertion that
	// pins the real bug: `docker compose up --no-deps admin` resolves only if
	// a service literally named "admin" exists, and it never does — docker
	// answers "no such service: admin".
	if _, ok := svcs["admin"]; ok {
		t.Error("generated compose defines a service named \"admin\"; " +
			"callers passing the image key would start working by accident and mask the real name")
	}
}

func TestAdminServiceDisabledByDefault(t *testing.T) {
	// Admin is TierOptional: a stack without the enable flag must not carry it.
	svcs := generatedServices(t, NewGenerator(minimalCfg()))

	if _, ok := svcs[AdminServiceName]; ok {
		t.Errorf("%q present without Admin.Enabled", AdminServiceName)
	}
}
