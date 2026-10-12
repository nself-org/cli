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
	onDisk, definedIn := scanEnvFiles(workdir, cfg.Env, sink)

	// Determine which secrets need persisting.
	var toWrite []secretEntry
	for _, s := range candidates {
		if s.value != "" && !onDisk[s.envKey] {
			toWrite = append(toWrite, s)
		}
	}

	// Core credentials the loader generates when unset (D-0506). Order per key:
	//  1. os.Getenv(key) != "": an active cascade file or a real export supplies
	//     it (the loader exports active files into the process env). Skip.
	//  2. A live value in .nself/compose.env is adopted for THIS build's render
	//     only and never written: compose.env (whichever env built last) cannot
	//     tell a generated value from one an earlier build took from a process
	//     export, and writing the latter would shadow a later rotated export.
	//     This holds even when an inactive env file defines the key, so a dev
	//     build does not lock out the running dev database.
	//  3. No env file defines it: persist the generated value (base64url from
	//     generateSecureRandom, so no quoting is needed).
	//  4. Only an inactive env file defines it and nothing is live: keep the
	//     generated value for this render, write nothing (.env.secrets is shared
	//     by every env and loads after .env.<env>, so a dev value written there
	//     would shadow the other env's real one), and warn. The next build
	//     adopts the value from compose.env, so it stays stable.
	for _, c := range []struct {
		key string
		dst *string
	}{
		{"POSTGRES_PASSWORD", &cfg.Postgres.Password},
		{"HASURA_GRAPHQL_ADMIN_SECRET", &cfg.Hasura.AdminSecret},
	} {
		if *c.dst == "" || os.Getenv(c.key) != "" {
			continue
		}
		if live, ok := adoptLiveCredential(sink, workdir, c.key); ok {
			*c.dst = live
			if !fx.Planning() {
				slog.Warn("key is not supplied by the active env files; reusing the value the running stack was built with. Pin it by adding the key to .env.secrets (or your env file)", "key", c.key)
			}
			continue
		}
		if onDisk[c.key] {
			if !fx.Planning() {
				slog.Warn("key is defined only in an env file that is not loaded for this env; this render uses a generated value. Set it in .env.<env> or .env.secrets to pin it",
					"key", c.key, "files", strings.Join(definedIn[c.key], ","), "env", cfg.Env)
			}
			continue
		}
		toWrite = append(toWrite, secretEntry{c.key, *c.dst})
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
