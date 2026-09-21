package config

// loader.go — env-cascade loader; entry point for config.Load().
//
// Purpose: Orchestrate the nSelf .env file cascade: read files in precedence
//          order, pass keys through warnUnknownEnvVars, then delegate to
//          parseEnvToConfig (loader_parse_env.go) for struct population and
//          ApplyDefaults (defaults.go) for smart defaults.
// Inputs:  projectDir string — the nSelf working directory that contains .env.*
//          files. ENV os env var controls which .env.{ENV} file is loaded.
// Outputs: *Config — fully populated and defaulted configuration struct.
// Constraints: Only Load() lives here. The env var list is in
//              loader_known_vars.go; field mapping in loader_parse_env.go;
//              collectPassthrough/parseInternalRoutes/parseExtensionList in
//              loader_helpers.go.
// SPORT:   cli/internal/config — decomposed from loader.go (T-E2-06).

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/version"

	"github.com/joho/godotenv"
)

// Load reads the .env cascade from projectDir, populates a Config struct from
// os.Getenv, applies smart defaults, and returns the complete configuration.
//
// Cascade order (later overrides earlier), approved 2026-08-23 GATE B
// (CLI-R18):
//
//	.env → .env.{dev|staging|prod} → .env.secrets → .env.local
//
// .env is the shared, committed base; exactly one of .env.dev/.env.staging/
// .env.prod loads, matching ENV; .env.secrets never ships in git; .env.local
// is the personal override and always wins. .env.ai no longer exists as a
// cascade layer — its content is folded into .env.secrets at init/upgrade
// (internal/setup/envai.go, internal/migrate/env_order.go).
//
// Setting NSELF_LEGACY_ENV_ORDER=1 restores the pre-CLI-R18 order for exactly
// one minor version (see EnvCascadeOrder in cascade.go for both orders in
// full), printing a warning on every use. Run `nself migrate` to move a
// project off the escape hatch permanently.
//
// Each file is optional. Missing files are silently skipped.
func Load(projectDir string) (*Config, error) {
	// 1. Detect ENV first (needed to pick the correct .env.{ENV} file).
	// Resolution order: process environment wins outright (an operator or
	// CI already exported ENV=...); otherwise the ENV= key inside the
	// project's own .env file (the shared, committed base every clone —
	// including a production server — carries) decides; otherwise "dev".
	// Without the .env fallback, a bare `nself build` run on a server whose
	// only signal is .env's ENV=prod silently built a dev compose instead
	// (wrong images, *.local.nself.org routes, mailpit/nself-admin wired in).
	env, envSource := ResolveEnv(projectDir)
	env = normalizeEnv(env)
	slog.Info("ENV resolved for .env cascade", "env", env, "source", envSource)

	// 2. Build the file cascade — canonical order unless the legacy escape
	// hatch is set.
	legacy := LegacyOrderActive()
	if legacy {
		warnLegacyEnvOrder()
	}
	names := EnvCascadeOrder(env, legacy)
	files := make([]string, 0, len(names))
	for _, n := range names {
		files = append(files, filepath.Join(projectDir, n))
	}

	// 3. Load each file (skip if not exists, later overrides earlier).
	// Simultaneously collect all keys that appear in any .env file so we can
	// warn about unknown vars after loading is complete.
	const maxEnvFileSize = 1 << 20 // 1 MB
	loadedFromFiles := make(map[string]string)
	for _, f := range files {
		info, statErr := os.Stat(f)
		if statErr != nil {
			continue // file doesn't exist, skip
		}
		if info.Size() > maxEnvFileSize {
			return nil, fmt.Errorf("config file too large (max 1MB): %s", f)
		}
		contents, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f, err)
		}
		if pos := bytes.IndexByte(contents, 0); pos >= 0 {
			return nil, fmt.Errorf("invalid .env file: contains null byte at position %d in %s", pos, f)
		}
		if err := godotenv.Overload(f); err != nil {
			return nil, fmt.Errorf("loading %s: %w", f, err)
		}
		// godotenv.Read parses the file without touching os.Environ, giving
		// us the raw key set to check for unknown vars.
		if m, err := godotenv.Read(f); err == nil {
			for k, v := range m {
				loadedFromFiles[k] = v
			}
		}
	}

	// Warn about env var names that are not in the known schema.
	// Warnings go to stderr via slog.Warn and never cause a non-zero exit.
	warnUnknownEnvVars(loadedFromFiles, knownEnvVars)

	// 4. Parse os.Environ into Config struct.
	cfg := parseEnvToConfig()

	// 5. Parse dynamic collections.
	customServices, err := parseCustomServices()
	if err != nil {
		return nil, err
	}
	cfg.CustomServices = customServices
	frontendApps, err := parseFrontendApps()
	if err != nil {
		return nil, err
	}
	cfg.FrontendApps = frontendApps
	remoteSchemas, err := parseRemoteSchemas()
	if err != nil {
		return nil, err
	}
	cfg.RemoteSchemas = remoteSchemas
	cfg.InternalRoutes = parseInternalRoutes()

	// 6. Collect passthrough vars (AUTH_PROVIDER_*, REMOTE_SCHEMA_*, HASURA_EXTRA_*).
	cfg.Passthrough = collectPassthrough(os.Environ())

	// 7. Apply smart defaults (fills every unset field).
	cfg, err = ApplyDefaults(cfg)
	if err != nil {
		return nil, fmt.Errorf("applying defaults: %w", err)
	}

	// 8. Sanitize user-supplied name and domain after defaults.
	if cfg.ProjectName != "" {
		sanitized, err := SanitizeName(cfg.ProjectName)
		if err != nil {
			return nil, fmt.Errorf("PROJECT_NAME: %w", err)
		}
		if sanitized != cfg.ProjectName {
			slog.Warn("PROJECT_NAME sanitized for Docker compatibility",
				"original", cfg.ProjectName,
				"sanitized", sanitized,
			)
		}
		cfg.ProjectName = sanitized
	}
	if cfg.BaseDomain != "" {
		sanitized, err := SanitizeDomain(cfg.BaseDomain)
		if err != nil {
			return nil, fmt.Errorf("BASE_DOMAIN: %w", err)
		}
		cfg.BaseDomain = sanitized
	}

	return cfg, nil
}

// ResolveEnv determines which environment name selects the .env.{ENV}
// cascade layer, and where that value came from (surfaced via slog and by
// `nself env explain`).
//
// Order: the process environment wins outright — an operator or CI that
// already exported ENV=... is always honored. Otherwise the ENV= key inside
// the project's own .env file (the shared, committed base present in every
// clone, including a bare production checkout with no shell env set) is
// read directly. Otherwise "dev".
//
// The .env fallback is read with a single targeted key lookup rather than
// running the full cascade, because the full cascade's file selection
// itself depends on the answer.
func ResolveEnv(projectDir string) (value string, source string) {
	if v := os.Getenv("ENV"); v != "" {
		return v, "process environment"
	}
	if v, ok := readEnvKeyFromFile(filepath.Join(projectDir, ".env"), "ENV"); ok && v != "" {
		return v, ".env"
	}
	return "dev", "default"
}

// readEnvKeyFromFile reads a single key's value out of a dotenv-format file
// without touching os.Environ or loading the rest of the file. Returns
// ok=false if the file is missing, unreadable, or does not define the key.
func readEnvKeyFromFile(path, key string) (value string, ok bool) {
	m, err := godotenv.Read(path)
	if err != nil {
		return "", false
	}
	v, present := m[key]
	return v, present
}

// warnLegacyEnvOrder logs a warning every time the NSELF_LEGACY_ENV_ORDER
// escape hatch is honored, naming the variable and the version in which it
// will stop being read (CLI-R18: the hatch is supported for exactly one
// minor version after the reorder ships).
func warnLegacyEnvOrder() {
	removalVersion := version.NextMinor(version.Version)
	slog.Warn(
		"NSELF_LEGACY_ENV_ORDER is set — using the pre-CLI-R18 .env cascade order. This escape hatch will be removed",
		"var", LegacyEnvOrderVar,
		"removed_in", removalVersion,
		"fix", "run 'nself migrate' to move this project to the new cascade order",
	)
}
