package compose

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestCoreServicesAreTheFourRequiredOnes pins the documented minimal stack.
// If a fifth service ever becomes mandatory, this fails and forces the docs,
// the wiki page and the install story to be updated with it.
func TestCoreServicesAreTheFourRequiredOnes(t *testing.T) {
	want := []string{"postgres", "hasura", "auth", "nginx"}
	got := CoreServices()

	if len(got) != len(want) {
		t.Fatalf("expected %d required services, got %d", len(want), len(got))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("required service %d = %q, want %q (boot order matters)", i, got[i].Name, name)
		}
	}
}

// TestRequiredServicesHaveNoEnableSwitch is the invariant that makes "required"
// mean something: a service you can turn off is optional by definition.
func TestRequiredServicesHaveNoEnableSwitch(t *testing.T) {
	for _, e := range CoreServices() {
		if e.EnableEnv != "" {
			t.Errorf("required service %q declares EnableEnv %q — it is optional, not required",
				e.Name, e.EnableEnv)
		}
	}
}

// TestOptionalServicesDeclareTheirGate is the mirror invariant: an optional
// service with no gate can never be turned on.
func TestOptionalServicesDeclareTheirGate(t *testing.T) {
	for _, e := range OptionalServices() {
		if e.EnableEnv == "" {
			t.Errorf("optional service %q declares no EnableEnv — nothing can enable it", e.Name)
		}
	}
}

// TestEveryCatalogEntryHasAPinnedImage keeps the catalog and the image pin map
// in step, so a documented service can never lack a default version.
func TestEveryCatalogEntryHasAPinnedImage(t *testing.T) {
	for _, e := range ServiceCatalog() {
		if e.DefaultImage == "" {
			t.Errorf("service %q has no image lock entry", e.Name)
		}
		if e.VersionEnv == "" {
			t.Errorf("service %q declares no VersionEnv — its image tag is unoverridable", e.Name)
		}
	}
}

// TestCatalogCoversGeneratorServices is the anti-drift guard. Every service the
// generator can add via AddService must be catalogued, or the published
// core-services page silently omits it.
//
// The generator's service names are extracted from the AddService call sites in
// this package rather than from a second hand-written list.
func TestCatalogCoversGeneratorServices(t *testing.T) {
	catalogued := map[string]bool{}
	for _, n := range CatalogNames() {
		catalogued[n] = true
	}

	// Services the generator adds that are deliberately outside the catalog:
	// init/sidecar containers and monitoring-stack members, which are covered
	// by SPORT F08 rather than by the core/optional split.
	exempt := map[string]bool{}
	for _, n := range monitoringStackServices() {
		exempt[n] = true
	}

	re := regexp.MustCompile(`AddService\("([a-z0-9_-]+)"`)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}

	var missing []string
	seen := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, m := range re.FindAllStringSubmatch(string(data), -1) {
			svc := m[1]
			if seen[svc] || catalogued[svc] || exempt[svc] || strings.HasSuffix(svc, "-init") {
				seen[svc] = true
				continue
			}
			seen[svc] = true
			missing = append(missing, svc+" (in "+name+")")
		}
	}

	if len(missing) > 0 {
		t.Fatalf("generator adds %d service(s) missing from the catalog — add them to "+
			"service_catalog.go or to the monitoring exemption:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// monitoringStackServices lists the observability sidecars that ship as a group
// and are documented in SPORT F08, not in the core/optional catalog.
func monitoringStackServices() []string {
	return []string{
		"prometheus", "grafana", "loki", "promtail", "tempo",
		"alertmanager", "cadvisor", "node-exporter", "postgres-exporter",
		"redis-exporter", "blackbox-exporter", "otel-collector",
	}
}
