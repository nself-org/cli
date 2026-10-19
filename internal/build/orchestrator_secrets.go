package build

// orchestrator_secrets.go — persists auto-generated secrets (plugin
// internal secrets, Hasura JWT secret, and the generated Postgres password and
// Hasura admin secret) to .env.secrets during build. Split from
// orchestrator.go (T-P6-E2-W1-S1-T3).
// Inputs:  workdir, resolved *config.Config.
// Outputs: error — writes .env.secrets as a side effect.
// Constraints: a generated credential must survive a rebuild (D-0506): the
// config loader generates POSTGRES_PASSWORD and HASURA_GRAPHQL_ADMIN_SECRET on
// every load when unset, so build 2 would render values that no longer match
// the running database. They are written once and read back by later loads.
// A value owned by the real process environment (CI export) is never written,
// because env files override the process env and would shadow a later rotation.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

func persistGeneratedSecrets(workdir string, cfg *config.Config) error {
	return persistGeneratedSecretsFx(workdir, cfg, writeEffects{}, newDiskSink(workdir))
}

// persistGeneratedSecretsFx is persistGeneratedSecrets with the .env.secrets
// append routed through the Sink (P7-LIVE-03): the new file content is the
// existing bytes plus the appended lines, written through sink in both modes so
// a plan carries the exact planned bytes of .env.secrets (and its plan_id binds
// them). The secrets-persist effect still records the key names, never values.
func persistGeneratedSecretsFx(workdir string, cfg *config.Config, fx Effects, sink Sink) error {
	type secretEntry struct {
		envKey string
		value  string
	}

	candidates := []secretEntry{
		{"PLUGIN_INTERNAL_SECRET", cfg.PluginSystem.InternalSecret},
		{"NOTIFY_INTERNAL_SECRET", cfg.PluginConfig.NotifySecret},
		{"CRON_INTERNAL_SECRET", cfg.PluginConfig.CronSecret},
	}

	// Collect env keys already present in any .env file on disk (or pending
	// in a plan's env overlay, P7-LIVE-05).
	onDisk := scanEnvFiles(workdir, cfg.Env, sink)

	// Determine which secrets need persisting.
	var toWrite []secretEntry
	for _, s := range candidates {
		if s.value != "" && !onDisk[s.envKey] {
			toWrite = append(toWrite, s)
		}
	}

	// Core credentials the loader generates when unset (D-0506). Persist a
	// freshly generated value only when no env file defines the key and the
	// real process environment does not supply it: that source owns the value,
	// and a file copy would shadow a later rotated export (files override the
	// process env on load). The onDisk term stays: .env.secrets is shared by
	// every env and loads after .env.<env>, so persisting a dev-generated value
	// while .env.prod (or any scanned env file) defines the key would shadow
	// that file's real value on the next build of that env.
	// A running database may already hold a value an earlier build rendered. It
	// is adopted for THIS build's render only (adoptLiveCredential) and never
	// written: .nself/compose.env cannot tell a generated value from one an
	// earlier build took from a process export, and writing the latter would
	// shadow a later rotated export. The next build adopts it again, because
	// compose.env then carries the same value. Generated values are base64url
	// (generateSecureRandom), so they need no quoting in .env.secrets.
	for _, c := range []struct {
		key string
		dst *string
	}{
		{"POSTGRES_PASSWORD", &cfg.Postgres.Password},
		{"HASURA_GRAPHQL_ADMIN_SECRET", &cfg.Hasura.AdminSecret},
	} {
		if *c.dst == "" || onDisk[c.key] || os.Getenv(c.key) != "" {
			continue
		}
		if live, ok := adoptLiveCredential(sink, workdir, c.key); ok {
			*c.dst = live
			if !fx.Planning() {
				slog.Warn("key is in no env file; reusing the value the running stack was built with. Pin it by adding the key to .env.secrets (or your env file)", "key", c.key)
			}
			continue
		}
		toWrite = append(toWrite, secretEntry{c.key, *c.dst})
	}

	// AUTH_ENCRYPTION_KEY for a new v1.5 project (D-0660), generated once.
	if key, ok, err := newAuthEncryptionKey(sink, workdir, cfg, onDisk); err != nil {
		return err
	} else if ok {
		toWrite = append(toWrite, secretEntry{compose.AuthEncryptionKeyEnv, key})
	}

	// HASURA_GRAPHQL_JWT_SECRET: Hasura is a core service, so if the JWT
	// secret is absent from every env file on disk, synthesize and persist it.
	// BuildJWTSecret auto-generates a crypto/rand key and returns the full
	// {"type":"HS256","key":"..."} JSON that Hasura expects.
	if !onDisk["HASURA_GRAPHQL_JWT_SECRET"] {
		jwtJSON, err := config.BuildJWTSecret(cfg)
		if err != nil {
			return fmt.Errorf("building JWT secret for persistence: %w", err)
		}
		if jwtJSON != "" {
			toWrite = append(toWrite, secretEntry{"HASURA_GRAPHQL_JWT_SECRET", jwtJSON})
		}
	}

	if len(toWrite) == 0 {
		return nil
	}

	keys := make([]string, 0, len(toWrite))
	for _, s := range toWrite {
		keys = append(keys, s.envKey)
	}
	secretsPath := filepath.Join(workdir, ".env.secrets")
	if err := fx.Do(EffectSecretsPersist, secretsPath, "append keys: "+strings.Join(keys, ", "), nil); err != nil {
		return err
	}
	existing, err := sink.ReadFile(secretsPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("opening %s: %w", secretsPath, err)
	}
	var buf bytes.Buffer
	buf.Write(existing)
	for _, s := range toWrite {
		fmt.Fprintf(&buf, "%s=%s\n", s.envKey, config.QuoteEnvValue(s.value))
		if !fx.Planning() {
			slog.Info("Persisted auto-generated secret to .env.secrets", "key", s.envKey)
		}
	}
	if err := sink.WriteFile(secretsPath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("writing %s: %w", secretsPath, err)
	}
	// Enforce 0600 unconditionally: the file may have existed with 0644.
	if err := sink.Chmod(secretsPath, 0600); err != nil {
		return fmt.Errorf("chmod %s: %w", secretsPath, err)
	}
	return nil
}

// The shared route model collects all nginx routes that will be generated for the
// given config. The list is used for preflight conflict detection via
// nginx.HasDomainConflict before any files are written.
//
// The server_name for each route is route + "." + baseDomain, matching the
// logic in the nginx Generator. Location is always "/" in current templates.
