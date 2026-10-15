package compose

// Purpose: share raw mounted credential values and restart versions (D-0551).
// Inputs: resolved config. Outputs: bytes and deterministic per-service digests.
// Constraints: generated YAML contains references, never the digest or value.
// SPORT: cli/compose secret transport.

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// SecretFileValues returns the exact bytes mounted by supported core services.
func SecretFileValues(cfg *config.Config) map[string]string {
	jwt, _ := config.BuildJWTSecret(cfg) // Generate validates Hasura's JWT config.
	return map[string]string{
		"POSTGRES_PASSWORD":           cfg.Postgres.Password,
		"MINIO_ROOT_USER":             cfg.Minio.RootUser,
		"MINIO_ROOT_PASSWORD":         cfg.Minio.RootPassword,
		"HASURA_GRAPHQL_ADMIN_SECRET": cfg.Hasura.AdminSecret,
		"HASURA_GRAPHQL_JWT_SECRET":   jwt,
		"HASURA_GRAPHQL_DATABASE_URL": fmt.Sprintf("postgresql://%s@%s:5432/%s", config.URLUserInfo(cfg.Postgres.User, cfg.Postgres.Password), cfg.Postgres.Host, cfg.Postgres.DB),
	}
}

// SecretFileVersions supplies Compose labels so an atomic file replacement
// recreates its consumers. Compose otherwise hashes only the constant path.
func SecretFileVersions(cfg *config.Config) map[string]string {
	g := NewGenerator(cfg)
	dc := &DockerCompose{Services: map[string]ServiceConfig{}}
	for _, name := range []string{"postgres", "minio", "hasura"} {
		dc.Services[name] = ServiceConfig{Environment: map[string]string{}}
	}
	g.fileSecrets(dc)
	values := SecretFileValues(cfg)
	versions := map[string]string{}
	for name, svc := range dc.Services {
		if len(svc.Secrets) == 0 {
			continue
		}
		h := sha256.New()
		sort.Strings(svc.Secrets)
		for _, key := range svc.Secrets {
			for _, part := range []string{key, values[strings.ToUpper(key)]} {
				_ = binary.Write(h, binary.BigEndian, uint64(len(part)))
				_, _ = h.Write([]byte(part))
			}
		}
		versions["NSELF_SECRET_VERSION_"+strings.ToUpper(name)] = fmt.Sprintf("%x", h.Sum(nil))
	}
	return versions
}
