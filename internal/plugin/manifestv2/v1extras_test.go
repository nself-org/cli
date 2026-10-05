package manifestv2_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

// v1Base is a v1 fixture without the keys that have no v2 home, so only the
// key under test can be reported as unmapped.
func v1Base(t *testing.T, set map[string]any) []byte {
	return mutate(t, "v1/ci.json", func(d map[string]any) {
		for _, k := range []string{"actions", "config", "hooks"} {
			delete(d, k)
		}
		for k, v := range set {
			d[k] = v
		}
	})
}

func unmappedNames(t *testing.T, data []byte) string {
	t.Helper()
	got, err := manifestv2.UnmappedV1Keys(data)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, u := range got {
		names = append(names, u.Key)
	}
	return strings.Join(names, ",")
}

// v1 routes (plugin API docs: method, path, auth, description, hmac, as in
// access-controls and nself-cloud) map to rest_routes, never to nginx routes.
func TestV1RoutesMapToRestRoutes(t *testing.T) {
	routes := []any{
		map[string]any{"method": "POST", "path": "/check", "auth": "bearer"},
		map[string]any{"method": "POST", "path": "/api/cloud/signup", "auth": "none", "description": "Create tenant + start trial. Rate: 5/IP/hour."},
		map[string]any{"method": "POST", "path": "/webhooks/stripe", "auth": "hmac", "hmac": "STRIPE_PLATFORM_WEBHOOK_SECRET"},
	}
	data := v1Base(t, map[string]any{"routes": routes})
	if got := unmappedNames(t, data); got != "" {
		t.Fatalf("routes must be mapped, unmapped = %q", got)
	}
	m, err := manifestv2.Normalize(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []manifestv2.RestRoute{
		{Method: "POST", Path: "/check", Auth: "bearer"},
		{Method: "POST", Path: "/api/cloud/signup", Auth: "none", Summary: "Create tenant + start trial. Rate: 5/IP/hour."},
		{Method: "POST", Path: "/webhooks/stripe", Auth: "hmac", HMAC: "STRIPE_PLATFORM_WEBHOOK_SECRET"},
	}
	if len(m.RestRoutes) != 3 || m.RestRoutes[0] != want[0] || m.RestRoutes[1] != want[1] || m.RestRoutes[2] != want[2] {
		t.Errorf("rest_routes = %+v", m.RestRoutes)
	}
	if len(m.Routes) != 0 {
		t.Errorf("v1 routes must never become nginx routes: %+v", m.Routes)
	}
	out, err := manifestv2.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(out, &doc)
	if _, bad := doc["routes"]; bad || doc["rest_routes"] == nil {
		t.Errorf("v2 file keys wrong: routes=%v rest_routes=%v", doc["routes"], doc["rest_routes"])
	}
	back, err := manifestv2.Parse(out)
	if err != nil || len(back.RestRoutes) != 3 || back.RestRoutes[2].HMAC == "" {
		t.Errorf("v2 round trip: %v %+v", err, back)
	}
}

// A routes value that does not convert losslessly stays unmapped (refused).
func TestV1RoutesUnconvertibleStayUnmapped(t *testing.T) {
	good := map[string]any{"method": "GET", "path": "/x", "auth": "bearer"}
	for name, set := range map[string]map[string]any{
		"extra key":        {"routes": []any{map[string]any{"method": "GET", "path": "/x", "upstream_port": 3000}}},
		"non-string":       {"routes": []any{map[string]any{"method": "GET", "path": 7}}},
		"not a list":       {"routes": map[string]any{"path": "/x"}},
		"with rest_routes": {"routes": []any{good}, "rest_routes": []any{map[string]any{"method": "GET", "path": "/y"}}},
	} {
		if got := unmappedNames(t, v1Base(t, set)); got != "routes" {
			t.Errorf("%s: unmapped = %q, want routes", name, got)
		}
	}
}

func TestV1CapabilitiesCarried(t *testing.T) {
	data := v1Base(t, map[string]any{"capabilities": []any{"crdt", "sync"}})
	if got := unmappedNames(t, data); got != "" {
		t.Fatalf("unmapped = %q", got)
	}
	m, err := manifestv2.Normalize(data)
	if err != nil || strings.Join(m.Capabilities, ",") != "crdt,sync" {
		t.Fatalf("%v %+v", err, m.Capabilities)
	}
	out, _ := manifestv2.Marshal(m)
	back, err := manifestv2.Parse(out)
	if err != nil || strings.Join(back.Capabilities, ",") != "crdt,sync" {
		t.Fatalf("v2 round trip: %v %+v", err, back.Capabilities)
	}
	if again, err := manifestv2.ApplyCompat(out); err != nil || string(again) != string(out) {
		t.Errorf("ApplyCompat changed the file: %v", err)
	}
	for _, bad := range []any{"crdt", []any{"a", 1}, map[string]any{"a": true}} {
		if got := unmappedNames(t, v1Base(t, map[string]any{"capabilities": bad})); got != "capabilities" {
			t.Errorf("capabilities %v: unmapped = %q", bad, got)
		}
	}
}

// env and env_vars map only in the {required: [name], optional: [name]} shape.
func TestV1EnvMapsOnlyWhenLossless(t *testing.T) {
	for _, key := range []string{"env", "env_vars"} {
		data := v1Base(t, map[string]any{key: map[string]any{"required": []any{"DATABASE_URL"}, "optional": []any{"LOG_LEVEL"}}})
		if got := unmappedNames(t, data); got != "" {
			t.Fatalf("%s: unmapped = %q", key, got)
		}
		m, err := manifestv2.Normalize(data)
		if err != nil || m.Env == nil || m.Env.Required[0] != "DATABASE_URL" || m.Env.Optional[0] != "LOG_LEVEL" {
			t.Fatalf("%s: %v %+v", key, err, m.Env)
		}
	}
	// Real shapes that carry defaults, descriptions or a flat map lose data in v2 env.
	lossy := map[string]any{
		"optional map with defaults (nself-sync)": map[string]any{"required": []any{"DATABASE_URL"}, "optional": map[string]any{"PORT": "3844"}},
		"list of objects (email)":                 []any{map[string]any{"key": "A", "required": true, "description": "x"}},
		"flat map (content-safety)":               map[string]any{"CONTENT_SAFETY_PLUGIN_PORT": "3213"},
		"list of names, no required flag":         []any{"A", "B"},
	}
	for name, v := range lossy {
		if got := unmappedNames(t, v1Base(t, map[string]any{"env": v})); got != "env" {
			t.Errorf("%s: unmapped = %q, want env", name, got)
		}
	}
	both := map[string]any{"required": []any{"A"}}
	if got := unmappedNames(t, v1Base(t, map[string]any{"env": both, "env_vars": both})); got != "env,env_vars" {
		t.Errorf("both keys: unmapped = %q", got)
	}
}

func TestIsMappedV1Key(t *testing.T) {
	for _, k := range []string{"name", "routes", "capabilities", "env", "env_vars", "bundles", "tier"} {
		if !manifestv2.IsMappedV1Key(k) {
			t.Errorf("%s should count as mapped", k)
		}
	}
	for _, k := range []string{"hooks", "actions", "notes", "entry"} {
		if manifestv2.IsMappedV1Key(k) {
			t.Errorf("%s should not count as mapped", k)
		}
	}
}
