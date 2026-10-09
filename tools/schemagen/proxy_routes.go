package main

import "github.com/nself-org/cli/internal/nginx/routemodel"

func init() {
	ovs := []Override{
		{Pointer: "/properties/_generated", Set: map[string]any{"const": routemodel.Generated}},
		{Pointer: "/properties/schema_version", Set: map[string]any{"const": "1"}},
		{Pointer: "/properties/default_server/properties/https/properties/action", Set: map[string]any{"enum": []string{"close", "not_found"}}},
		{Pointer: "/properties/zones/items/properties/kind", Set: map[string]any{"enum": []string{"req", "conn"}}},
		{Pointer: "/properties/routes/items/properties/source", Set: map[string]any{"enum": []string{"core", "optional", "custom_service", "frontend", "internal", "plugin", "hand_managed"}}},
		{Pointer: "/properties/routes/items/properties/locations/items/properties/match", Set: map[string]any{"enum": []string{"prefix", "exact"}}},
		{Pointer: "/properties/routes/items/properties/locations/items/properties/upstream/properties/scheme", Set: map[string]any{"enum": []string{"http", "https"}}},
	}
	for _, p := range []string{
		"/properties/unmodelled_global", "/properties/zones", "/properties/routes",
		"/properties/defaults/properties/gzip/properties/types", "/properties/defaults/properties/tls/properties/protocols",
		"/properties/routes/items/properties/server_names", "/properties/routes/items/properties/blocked_paths",
		"/properties/routes/items/properties/locations", "/properties/routes/items/properties/unmodelled",
		"/properties/routes/items/properties/tls/properties/protocols",
	} {
		ovs = append(ovs, Override{Pointer: p, Set: map[string]any{"type": "array"}})
	}
	for _, p := range []string{
		"/properties/routes/items/properties/security_headers",
		"/properties/routes/items/properties/locations/items/properties/headers_set",
	} {
		ovs = append(ovs, Override{Pointer: p, Set: map[string]any{"type": "object"}})
	}
	Register(Spec{Out: "proxy-routes.v1.schema.json", Type: routemodel.Model{}, Overrides: ovs})
}
