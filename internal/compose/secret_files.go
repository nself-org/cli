package compose

// Purpose: remove supported credentials from Docker Config.Env (D-0551).
// Inputs: generated services and their resolved config. Outputs: file grants.
// Constraints: preserve image security settings and whitespace-sensitive values.
// SPORT: cli/compose secret transport.

import (
	"sort"
	"strings"
)

// fileSecrets runs after service generation so builders retain their curated
// values and passthrough precedence before the transport is selected.
func (g *Generator) fileSecrets(dc *DockerCompose) {
	dc.Secrets = map[string]SecretConfig{}
	for _, name := range []string{"postgres", "minio", "hasura"} {
		svc, exists := dc.Services[name]
		if !exists {
			continue
		}
		var keys []string
		switch name {
		case "postgres":
			// postgres's file_env strips trailing newlines.
			if !strings.HasSuffix(g.cfg.Postgres.Password, "\n") {
				keys = []string{"POSTGRES_PASSWORD"}
			}
		case "minio":
			for key, value := range map[string]string{"MINIO_ROOT_USER": g.cfg.Minio.RootUser, "MINIO_ROOT_PASSWORD": g.cfg.Minio.RootPassword} {
				// MinIO trims whitespace when reading a credential file.
				if strings.TrimSpace(value) == value {
					keys = append(keys, key)
				}
			}
		case "hasura":
			keys = []string{"HASURA_GRAPHQL_ADMIN_SECRET", "HASURA_GRAPHQL_JWT_SECRET", "HASURA_GRAPHQL_DATABASE_URL"}
			svc.Command = []string{"/bin/sh", "-ec", hasuraFileCommand(keys)}
		}
		for _, key := range keys {
			delete(svc.Environment, key)
			svc.Secrets = append(svc.Secrets, strings.ToLower(key))
			dc.Secrets[strings.ToLower(key)] = SecretConfig{File: "./.nself/secrets/" + key}
			if name != "hasura" {
				svc.Environment[key+"_FILE"] = "/run/secrets/" + strings.ToLower(key)
			}
		}
		sort.Strings(svc.Secrets)
		if len(svc.Secrets) > 0 {
			if svc.Labels == nil {
				svc.Labels = map[string]string{}
			}
			svc.Labels["io.nself.secret-version"] = "${NSELF_SECRET_VERSION_" + strings.ToUpper(name) + "}"
		}
		dc.Services[name] = svc
	}
}

// hasuraFileCommand reads at runtime, never interpolating credentials into
// Compose argv. A sentinel preserves trailing newlines. The image's original
// CMD is "$HGE_BINARY serve" in both supported Hasura image versions.
func hasuraFileCommand(keys []string) string {
	var script strings.Builder
	for _, key := range keys {
		script.WriteString("value=$$(cat /run/secrets/" + strings.ToLower(key) + " && printf .); export " + key + "=\"$${value%.}\"; ")
	}
	script.WriteString("exec \"$$HGE_BINARY\" serve")
	return script.String()
}
