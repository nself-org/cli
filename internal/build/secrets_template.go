package build

// Purpose: Keep secrets OUT of the generated docker-compose.yml. nself build
//          previously baked literal passwords/keys (postgres, Hasura admin
//          secret, JWT blobs, MinIO root creds, ...) into the generated YAML,
//          so editing .env was a silent no-op and any commit of the generated
//          file leaked credentials (PCI compose-secret-templating, 2026-07-03;
//          ASI generated-file-secret hard rule; ntask incident 085af2ec).
// Inputs:  resolved *config.Config + generated compose YAML bytes.
// Outputs: compose YAML with ${VAR} references instead of literals, plus the
//          .nself/compose.env interpolation file (0600) that `nself start`
//          passes to docker compose via --env-file.
// Constraints: env-key lines first, then URL-credential positions
//              (":<pw>@" → ${VAR}; percent-encoded ":<enc>@" →
//              ${VAR_URLENC:-${VAR}}), then any other literal. No literal may
//              stay: compose interpolates it, so a "$" in a secret would
//              resolve differently per consumer (D-0635). Anything left is
//              reported by LiteralSecretLeaks.
// SPORT: cli/internal/build — secret templating (S-secret-env-templating).

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/composeenv"
	"github.com/nself-org/cli/internal/config"
)

// composeEnvFile is the path (relative to workdir) of the interpolation env
// file consumed by every `docker compose` invocation the CLI makes.
const composeEnvFile = ".nself/compose.env"

// SecretEnvMap returns env-var-name → resolved literal value for every secret
// the compose generator may embed, plus the plugin internal secrets every
// plugin fragment reads as ${VAR} (D-0647: compose.env must carry the value
// the loader generated and .env.secrets persisted, even when .env lacks it).
// Empty values are omitted.
func SecretEnvMap(cfg *config.Config) map[string]string {
	m := make(map[string]string)
	add := func(key, val string) {
		if strings.TrimSpace(val) != "" {
			m[key] = val
		}
	}

	add("POSTGRES_PASSWORD", cfg.Postgres.Password)
	add("HASURA_GRAPHQL_ADMIN_SECRET", cfg.Hasura.AdminSecret)
	add("HASURA_JWT_KEY", cfg.Hasura.JWTKey)
	if jwtJSON, err := config.BuildJWTSecret(cfg); err == nil {
		add("HASURA_GRAPHQL_JWT_SECRET", jwtJSON)
	}
	if cfg.Minio.Enabled {
		add("MINIO_ROOT_USER", cfg.Minio.RootUser)
		add("MINIO_ROOT_PASSWORD", cfg.Minio.RootPassword)
	}
	add("S3_ACCESS_KEY", cfg.Minio.S3AccessKey)
	add("S3_SECRET_KEY", cfg.Minio.S3SecretKey)
	add("REDIS_PASSWORD", cfg.Redis.Password)
	add("AUTH_SMTP_PASS", cfg.Auth.SMTPPass)
	add("MAILPIT_UI_PASSWORD", cfg.Mailpit.UIPassword)
	add("ADMIN_SECRET_KEY", cfg.Admin.SecretKey)
	add("ADMIN_PASSWORD_HASH", cfg.Admin.PasswordHash)
	add("GRAFANA_ADMIN_PASSWORD", cfg.Monitoring.GrafanaAdminPassword)
	add("SEARCH_API_KEY", cfg.Search.APIKey)
	add("MEILISEARCH_MASTER_KEY", cfg.Search.MeiliSearch.MasterKey)
	add("TYPESENSE_API_KEY", cfg.Search.Typesense.APIKey)
	add("PLUGIN_INTERNAL_SECRET", cfg.PluginSystem.InternalSecret)
	add("NOTIFY_INTERNAL_SECRET", cfg.PluginConfig.NotifySecret)

	return m
}

// secretAliasKeys maps additional compose env keys that carry a mapped
// secret's value under a different name. The alias line is rewritten to the
// canonical ${VAR} only when the line actually contains the literal value.
var secretAliasKeys = map[string]string{
	"AUTH_JWT_SECRET":        "HASURA_JWT_KEY",
	"SHARED_AUTH_JWT_SECRET": "HASURA_JWT_KEY",
	"AUTH_DB_PASSWORD":       "POSTGRES_PASSWORD", // Nhost Auth AUTH_DB_* alias set
}

// urlEncodedTwinKeys are the passwords WriteComposeEnv also writes
// percent-encoded as <KEY>_URLENC, for compose references inside a URL.
var urlEncodedTwinKeys = []string{"POSTGRES_PASSWORD", "REDIS_PASSWORD"}

// twinSource returns the configured password behind a urlEncodedTwinKeys entry.
func twinSource(cfg *config.Config, key string) string {
	switch key {
	case "POSTGRES_PASSWORD":
		return cfg.Postgres.Password
	case "REDIS_PASSWORD":
		return cfg.Redis.Password
	}
	return ""
}

// TemplateSecrets rewrites generated compose YAML so no literal secret value
// remains: env entries become ${VAR} references and URL-embedded database
// credentials become :${VAR}@. Docker Compose interpolates the references at
// container-start time from --env-file (.nself/compose.env), so secrets never
// land in the generated (potentially committed) YAML.
func TemplateSecrets(composeYAML []byte, secrets map[string]string) []byte {
	if len(secrets) == 0 {
		return composeYAML
	}
	out := string(composeYAML)

	// Phase A: exact env-key lines → "KEY: ${KEY}".
	// The compose generator emits env maps as "      KEY: value"; the whole
	// value is replaced regardless of YAML quoting style.
	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = rewriteKeyLines(out, key, func(line string) (string, bool) {
			// The generator may deliberately select a different credential source
			// (CS_N S3 keys use MinIO root credentials). Preserve explicit refs.
			if ref := composeRefPattern.FindString(line); ref != "" && lineValueIs(line, ref) {
				return "", false
			}
			return "${" + key + "}", true
		})
	}

	// Phase A2: alias keys (e.g. AUTH_JWT_SECRET carries HASURA_JWT_KEY's
	// value). Rewritten only when the line still holds the literal value, so
	// aliases with independent values are never clobbered.
	aliasNames := make([]string, 0, len(secretAliasKeys))
	for alias := range secretAliasKeys {
		aliasNames = append(aliasNames, alias)
	}
	sort.Strings(aliasNames)
	for _, alias := range aliasNames {
		canonical := secretAliasKeys[alias]
		val, ok := secrets[canonical]
		if !ok {
			continue
		}
		out = rewriteKeyLines(out, alias, func(line string) (string, bool) {
			if !strings.Contains(line, val) && !lineValueIs(line, val) {
				return "", false
			}
			return "${" + canonical + "}", true
		})
	}

	// Phase B: URL credential positions — ":<password>@" → ":${VAR}@".
	// Covers DATABASE_URL / HASURA_GRAPHQL_DATABASE_URL / redis URLs where the
	// password is embedded mid-value. Only safe when the password needs no
	// percent-encoding (raw == PathEscape(raw)); otherwise interpolation would
	// inject an unescaped value into a URL, so the literal is left in place
	// and surfaced by LiteralSecretLeaks.
	for _, key := range keys {
		val := secrets[key]
		if len(val) < 4 || url.PathEscape(val) != val {
			continue
		}
		out = strings.ReplaceAll(out, ":"+val+"@", ":${"+key+"}@")
	}

	// Phase B2: percent-encoded credential positions — ":<URLPassword(pw)>@"
	// → ":${VAR_URLENC:-${VAR}}@" for every password WriteComposeEnv writes
	// an encoded twin for. Phase B leaves these literal, and docker compose
	// interpolates the literal: a "$" in it was expanded or rejected while
	// ${VAR} consumers read the verbatim password (D-0635). The twin holds the
	// exact encoded bytes, so the resolved URL is unchanged for every other
	// password, and the fallback matches the fragment form (P7-PROD-31).
	for _, key := range urlEncodedTwinKeys {
		val, ok := secrets[key]
		if !ok {
			continue
		}
		enc := config.URLPassword(val)
		out = strings.ReplaceAll(out, ":"+enc+"@", ":${"+key+config.URLPasswordEnvSuffix+":-${"+key+"}}@")
	}

	// Phase C: any other literal occurrence (secrets_template_rewrite.go).
	out = templateOtherLiterals(out, keys, secrets)

	return []byte(out)
}

// WriteComposeEnv writes .nself/compose.env (0600) — the single interpolation
// file that resolves every ${VAR} reference in the generated compose set:
// secrets, Hasura console/dev-mode toggles, computed values (DATABASE_URL,
// DOCKER_NETWORK), and plugin env vars. CLI compose calls on the project
// pass it via --env-file: ComposeEnvFiles, or docker.ProjectEnvFiles where
// internal/build cannot be imported. Sites that still shell out to docker
// compose directly (the repoqa docker-exec allowlist) may not; hq debt
// D-0747 tracks them. Every value goes through
// config.QuoteComposeEnvValue, so compose reads back the exact bytes even
// for secrets holding whitespace, '#', '$', quotes or backslashes (D-0333).
func WriteComposeEnv(workdir string, cfg *config.Config, secrets, pluginEnvVars map[string]string) error {
	return writeComposeEnvVia(newDiskSink(workdir), workdir, cfg, secrets, pluginEnvVars)
}

// writeComposeEnvVia is WriteComposeEnv writing through sink.
func writeComposeEnvVia(sink Sink, workdir string, cfg *config.Config, secrets, pluginEnvVars map[string]string) error {
	merged := make(map[string]string, len(secrets)+len(pluginEnvVars)+4)
	for k, v := range pluginEnvVars {
		merged[k] = v
	}
	for k, v := range secrets {
		merged[k] = v
	}

	network := cfg.DockerNetwork
	if network == "" {
		network = cfg.ProjectName + "_network"
	}
	merged["DATABASE_URL"] = cfg.DatabaseURL()
	merged["DOCKER_NETWORK"] = network
	// URL-encoded twins for compose fragments that write a password into a URL
	// (P7-PROD-31): compose can substitute but not encode. Encoded exactly once
	// here; fragments use ${X_URLENC:-${X}} so an older compose.env without the
	// twin renders what it always rendered. Empty passwords get no twin.
	for _, key := range urlEncodedTwinKeys {
		if enc := config.URLPassword(twinSource(cfg, key)); enc != "" {
			merged[key+config.URLPasswordEnvSuffix] = enc
		}
	}
	merged["HASURA_GRAPHQL_ENABLE_CONSOLE"] = fmt.Sprintf("%t", cfg.Hasura.Console)
	merged["HASURA_GRAPHQL_DEV_MODE"] = fmt.Sprintf("%t", cfg.Hasura.DevMode)

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("# GENERATED BY nself build - DO NOT HAND EDIT\n")
	sb.WriteString("# Interpolation values for the ${VAR} references in docker-compose.yml.\n")
	sb.WriteString("# Passed to docker compose via --env-file by nself start/stop/restart.\n")
	sb.WriteString(composeenv.Marker + composeenv.EnvName(cfg.Env) + "\n") // D-1149
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(config.QuoteComposeEnvValue(merged[k]))
		sb.WriteString("\n")
	}

	path := filepath.Join(workdir, composeEnvFile)
	if err := sink.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s dir: %w", filepath.Dir(composeEnvFile), err)
	}
	if err := sink.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", composeEnvFile, err)
	}
	// Enforce 0600 even when the file pre-existed with a looser mode.
	if err := sink.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", composeEnvFile, err)
	}
	return nil
}

// ComposeEnvFiles returns the ordered --env-file list for docker compose
// invocations in workdir: the project .env (when present, for user overrides
// and backward compatibility) followed by .nself/compose.env (computed truth,
// wins on conflict). Returns nil when neither exists — legacy projects built
// before secret templating keep working unchanged.
func ComposeEnvFiles(workdir string) []string {
	var files []string
	for _, rel := range []string{".env", composeEnvFile} {
		p := filepath.Join(workdir, rel)
		if _, err := os.Stat(p); err == nil {
			files = append(files, p)
		}
	}
	// The default project .env is only useful alongside compose.env; docker
	// compose reads it implicitly anyway when it is the sole file present.
	// Compare whole paths: a suffix test against the slash-form composeEnvFile
	// never matched a Windows path, so a compose.env-only project got nil.
	if len(files) == 1 && files[0] == filepath.Join(workdir, ".env") {
		return nil
	}
	return files
}
