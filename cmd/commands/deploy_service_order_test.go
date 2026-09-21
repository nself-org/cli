package commands

// deploy_service_order_test.go — regression coverage for the rolling-restart
// service-order derivation (deploy_service_order.go). See that file's header
// for the production incident: a fixed [postgres hasura auth storage
// plugins] list restarted a nonexistent "storage" service against a real
// project whose compose only had "minio", aborting the deploy after
// postgres/hasura/auth had already been recreated.

import (
	"reflect"
	"testing"
)

// TestResolveServiceOrder_RealProjectCompose reproduces the exact production
// compose that triggered the incident: postgres hasura auth auth-server
// mailpit minio ping-api nginx redis nself-admin — no "storage", no
// "plugins". The old fixed deployServiceOrder would have named "storage",
// which never exists in any project's compose.
func TestResolveServiceOrder_RealProjectCompose(t *testing.T) {
	present := []string{
		"postgres", "hasura", "auth", "auth-server", "mailpit",
		"minio", "ping-api", "nginx", "redis", "nself-admin",
	}
	got := resolveServiceOrder(present, nil)

	// Every name returned must actually be one docker compose reported —
	// never a name like "storage" that doesn't exist in this compose.
	presentSet := make(map[string]bool, len(present))
	for _, s := range present {
		presentSet[s] = true
	}
	for _, s := range got {
		if !presentSet[s] {
			t.Errorf("resolveServiceOrder returned %q, which is not in the project's compose", s)
		}
	}
	if len(got) != len(present) {
		t.Fatalf("expected every present service to appear exactly once; got %v", got)
	}

	// Core order must come first, in dependency order.
	want := []string{"postgres", "hasura", "auth"}
	if !reflect.DeepEqual(got[:3], want) {
		t.Errorf("expected core prefix %v, got %v", want, got[:3])
	}
}

// TestResolveServiceOrder_FakeComposeMinioNoStorage is the exact scenario
// named in the fix: a fake compose listing minio and ping-api, and no
// "storage" service at all.
func TestResolveServiceOrder_FakeComposeMinioNoStorage(t *testing.T) {
	present := []string{"postgres", "hasura", "auth", "minio", "ping-api"}
	got := resolveServiceOrder(present, nil)

	for _, s := range got {
		if s == "storage" {
			t.Fatalf("resolveServiceOrder must never invent a %q service that isn't present: %v", s, got)
		}
	}
	if !contains(got, "minio") || !contains(got, "ping-api") {
		t.Errorf("expected minio and ping-api in the resolved order, got %v", got)
	}
}

// TestResolveServiceOrder_CoreOnlyWhenPresent verifies that a core service
// absent from the compose (e.g. a project with no "auth" service) is never
// injected into the order.
func TestResolveServiceOrder_CoreOnlyWhenPresent(t *testing.T) {
	present := []string{"postgres", "hasura", "nginx"}
	got := resolveServiceOrder(present, nil)

	if contains(got, "auth") {
		t.Errorf("auth is not present in this project's compose but appeared in the order: %v", got)
	}
	want := []string{"postgres", "hasura", "nginx"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestResolveServiceOrder_PluginsLast verifies plugin-contributed services
// always sort after every core and non-plugin service, preserving their
// relative order within each group.
func TestResolveServiceOrder_PluginsLast(t *testing.T) {
	present := []string{"plugin-b", "postgres", "nginx", "hasura", "plugin-a", "auth", "minio"}
	pluginServices := map[string]bool{"plugin-a": true, "plugin-b": true}

	got := resolveServiceOrder(present, pluginServices)
	want := []string{"postgres", "hasura", "auth", "nginx", "minio", "plugin-b", "plugin-a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestResolveServiceOrder_EmptyPresent verifies an empty compose produces an
// empty order rather than falling back to any fixed guess list.
func TestResolveServiceOrder_EmptyPresent(t *testing.T) {
	got := resolveServiceOrder(nil, nil)
	if len(got) != 0 {
		t.Errorf("expected empty order for empty present list, got %v", got)
	}
}

// TestResolveServiceOrder_NoDuplicates verifies a service listed once in
// present never appears twice even if it happens to also be flagged as a
// plugin service (defensive: core/plugin sets should never legitimately
// overlap, but the function must not double-restart a service either way).
func TestResolveServiceOrder_NoDuplicates(t *testing.T) {
	present := []string{"postgres", "hasura", "auth"}
	pluginServices := map[string]bool{"postgres": true}

	got := resolveServiceOrder(present, pluginServices)
	seen := map[string]int{}
	for _, s := range got {
		seen[s]++
	}
	for s, n := range seen {
		if n != 1 {
			t.Errorf("service %q appeared %d times, want 1: %v", s, n, got)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
