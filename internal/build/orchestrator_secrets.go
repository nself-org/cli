package build

// orchestrator_secrets.go — persists auto-generated secrets (plugin
// internal secrets, Hasura JWT secret) to .env.secrets during build.
// Split from orchestrator.go (T-P6-E2-W1-S1-T3).
// Inputs:  workdir, resolved *config.Config.
// Outputs: error — writes .env.secrets as a side effect.
// Constraints: pure move, same checks/output/errors, no behavior change.

import (
	"bufio"
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

	// Collect env keys already present in any .env file on disk.
	onDisk := make(map[string]bool)
	for _, name := range []string{".env", ".env.dev", ".env.staging", ".env.prod", ".env.secrets", ".env.local"} {
		p := filepath.Join(workdir, name)
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || line[0] == '#' {
				continue
			}
			if idx := strings.IndexByte(line, '='); idx > 0 {
				onDisk[line[:idx]] = true
			}
		}
		_ = f.Close()
	}

	// Determine which secrets need persisting.
	var toWrite []secretEntry
	for _, s := range candidates {
		if s.value != "" && !onDisk[s.envKey] {
			toWrite = append(toWrite, s)
		}
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

// buildNginxRoutes collects all nginx routes that will be generated for the
// given config. The list is used for preflight conflict detection via
// nginx.HasDomainConflict before any files are written.
//
// The server_name for each route is route + "." + baseDomain, matching the
// logic in the nginx Generator. Location is always "/" in current templates.
