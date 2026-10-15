package build

// Purpose: Persist generated credentials blocked by another environment's key
// into the active environment's own file (D-0707, lead-approved alternative).
// Inputs: resolved config, all-env key scan, remote flag, build effects/sink.
// Outputs: an append to .env.<env> (0600), or its planned equivalent.
// Constraints: keep the shared .env.secrets guard; never copy configured keys
// or compose.env values across environments. No cascade layer is introduced.
// SPORT: cli/internal/build — generated secret persistence.

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/nself-org/cli/internal/config"
)

func persistInactiveSecretsFx(workdir string, cfg *config.Config, onDisk map[string]bool, fx Effects, sink Sink, remote bool) error {
	canonical := config.EnvCascadeOrder(cfg.Env, false)
	// Invalid environment names have no environment-specific layer.
	if len(canonical) != 4 {
		return nil
	}
	name := canonical[1]
	if slices.Contains([]string{".env.secrets", ".env.local", ".env.ai", ".env.example", ".env.computed"}, name) {
		return nil // shared, template and generated files are not per-env storage
	}
	order := config.EnvCascadeOrder(cfg.Env, config.LegacyOrderActive())
	if remote {
		order = config.WithoutLocalOverride(order)
	}
	if !slices.Contains(order, name) {
		return nil // e.g. a custom environment under the legacy cascade
	}
	active := activeSecretKeys(workdir, order, sink)
	needsWrite := func(key string) bool {
		return onDisk[key] && !active[key] && os.Getenv(key) == ""
	}
	entries := []secretEntry{
		{"POSTGRES_PASSWORD", cfg.Postgres.Password},
		{"HASURA_GRAPHQL_ADMIN_SECRET", cfg.Hasura.AdminSecret},
		{"PLUGIN_INTERNAL_SECRET", cfg.PluginSystem.InternalSecret},
		{"NOTIFY_INTERNAL_SECRET", cfg.PluginConfig.NotifySecret},
		{"CRON_INTERNAL_SECRET", cfg.PluginConfig.CronSecret},
	}
	var pending []secretEntry
	for _, entry := range entries {
		if entry.value != "" && needsWrite(entry.envKey) {
			pending = append(pending, entry)
		}
	}
	if needsWrite("HASURA_GRAPHQL_JWT_SECRET") && !configuredJWT(active) {
		jwt, err := config.BuildJWTSecret(cfg)
		if err != nil {
			return err
		}
		if jwt != "" {
			pending = append(pending, secretEntry{"HASURA_GRAPHQL_JWT_SECRET", jwt})
		}
	}
	return appendGeneratedSecretsFx(workdir, name, pending, fx, sink)
}

// activeSecretKeys reads the effective cascade, including pending plan inputs.
// Inputs: workdir, ordered active filenames, sink. Outputs: configured key set.
// Constraints: read-only, no values logged; unreadable files follow scanEnvFiles.
func activeSecretKeys(workdir string, order []string, sink Sink) map[string]bool {
	keys := make(map[string]bool)
	for _, name := range order {
		data, err := envLayerBytes(sink, filepath.Join(workdir, name))
		if err != nil {
			continue
		}
		values, err := config.ParseEnvBytes(name, data)
		if err != nil {
			continue // the config loader reports malformed active env files
		}
		for key := range values {
			keys[key] = true
		}
	}
	return keys
}

// configuredJWT recognizes the key aliases that own an explicitly set JWT.
// Inputs: active key set plus process env. Outputs: whether JWT is configured.
// Constraints: do not persist a copy that would shadow later alias rotations.
func configuredJWT(active map[string]bool) bool {
	for _, key := range []string{"HASURA_JWT_KEY", "AUTH_JWT_SECRET", "AUTH_JWT_KEY"} {
		if active[key] || os.Getenv(key) != "" {
			return true
		}
	}
	return false
}
