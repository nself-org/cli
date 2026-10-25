// Schema registration: shared host proxy registration (contract:cli.shared-proxy
// v1, P7-ADOPT-03).
//
// Purpose: schemas/shared-proxy.v1.schema.json is generated from
// sharedproxy.Registration. Lists are never null in a written file (Marshal
// replaces nil with []), so the schema says `array`.
package main

import "github.com/nself-org/cli/internal/sharedproxy"

func init() {
	ovs := []Override{
		{Pointer: "/properties/_generated", Set: map[string]any{"const": sharedproxy.Generated}},
		{Pointer: "/properties/schema_version", Set: map[string]any{"const": sharedproxy.SchemaVersion}},
		{Pointer: "/properties/files/items", Set: map[string]any{"pattern": `^(nginx/sites/[^/]+|ssl/certificates/[^/]+/(fullchain|privkey)\.pem)$`}},
	}
	for _, p := range []string{"/properties/files", "/properties/server_names", "/properties/service_names", "/properties/verbatim_upstream_hosts"} {
		ovs = append(ovs, Override{Pointer: p, Set: map[string]any{"type": "array"}})
	}
	Register(Spec{Out: "shared-proxy.v1.schema.json", Type: sharedproxy.Registration{}, Overrides: ovs})
}
