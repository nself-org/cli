package build

// Purpose: recognize only the generator's constrained file mounts in the
// socket policy (D-0551). Inputs: parsed core document and service grants.
// Outputs: true only for known credentials at build-owned paths.
// Constraints: plugin/override grants and arbitrary secret sources remain opaque.
// SPORT: cli/build socket inspection.

import "strings"

func generatedCoreSecrets(d socketDoc, service string, grants any) bool {
	if d.origin != "core stack" {
		return false
	}
	allowed := map[string][]string{
		"postgres": {"POSTGRES_PASSWORD"},
		"minio":    {"MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD"},
		"hasura":   {"HASURA_GRAPHQL_ADMIN_SECRET", "HASURA_GRAPHQL_JWT_SECRET", "HASURA_GRAPHQL_DATABASE_URL"},
	}
	list, ok := grants.([]any)
	if !ok {
		return false
	}
	defs, ok := d.top["secrets"].(map[string]any)
	if !ok {
		return false
	}
	for _, item := range list {
		name, ok := item.(string)
		if !ok {
			return false
		}
		known := false
		for _, key := range allowed[service] {
			if name == strings.ToLower(key) {
				def, ok := defs[name].(map[string]any)
				if !ok || len(def) != 1 || def["file"] != "./"+composeSecretsDir+"/"+key {
					return false
				}
				known = true
			}
		}
		if !known {
			return false
		}
	}
	return true
}
