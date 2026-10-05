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
// Constraints: Two-phase rewrite — exact env-key lines first, then
//              URL-credential positions (":<pw>@"). URL rewrite only when the
//              password needs no percent-encoding (raw == escaped), otherwise
//              the literal is left and reported by LiteralSecretLeaks.
// SPORT: cli/internal/build — secret templating (S-secret-env-templating).

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// composeEnvFile is the path (relative to workdir) of the interpolation env
// file consumed by every `docker compose` invocation the CLI makes.
const composeEnvFile = ".nself/compose.env"

// SecretEnvMap returns env-var-name → resolved literal value for every secret
// the compose generator may embed. Empty values are omitted.
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
	add("MINIO_ROOT_USER", cfg.Minio.RootUser)
	add("MINIO_ROOT_PASSWORD", cfg.Minio.RootPassword)
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
		re := regexp.MustCompile(`(?m)^(\s+)` + regexp.QuoteMeta(key) + `:\s.*$`)
		// $$ emits a literal $ in Go regexp replacement templates — the
		// output must contain the un-expanded ${KEY} for docker compose.
		out = re.ReplaceAllString(out, "${1}"+key+": $${"+key+"}")
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
		re := regexp.MustCompile(`(?m)^(\s+)` + regexp.QuoteMeta(alias) + `:\s.*$`)
		out = re.ReplaceAllStringFunc(out, func(line string) string {
			if !strings.Contains(line, val) {
				return line
			}
			indent := line[:strings.Index(line, alias)]
			return indent + alias + ": ${" + canonical + "}"
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

	// Phase C: any other literal occurrence — service generators sometimes
	// embed a secret in a `command:` string or a healthcheck `test:` argument
	// (e.g. Redis's `--requirepass <pw>`, Typesense's `--api-key=<key>` and
	// its healthcheck header) rather than a plain "KEY: value" env line or a
	// URL credential position, so Phases A/B never see them. Safe to replace
	// unconditionally for values that need no percent-encoding — those are
	// exactly the values compose interpolation can substitute verbatim
	// anywhere. Percent-encoding-sensitive values are left literal
	// everywhere (same rationale as Phase B) and still caught by
	// LiteralSecretLeaks as a defense-in-depth warning.
	for _, key := range keys {
		val := secrets[key]
		if len(val) < 8 || url.PathEscape(val) != val {
			continue
		}
		out = strings.ReplaceAll(out, val, "${"+key+"}")
	}

	return []byte(out)
}

// LiteralSecretLeaks returns the names of secrets whose literal value (>= 8
// chars) still appears in the compose YAML after templating. Callers surface
// each as a build warning — the safety net behind TemplateSecrets.
func LiteralSecretLeaks(composeYAML []byte, secrets map[string]string) []string {
	var leaked []string
	doc := string(composeYAML)
	for key, val := range secrets {
		if len(val) < 8 {
			continue
		}
		if strings.Contains(doc, val) {
			leaked = append(leaked, key)
		}
	}
	sort.Strings(leaked)
	return leaked
}

// WriteComposeEnv writes .nself/compose.env (0600) — the single interpolation
// file that resolves every ${VAR} reference in the generated compose set:
// secrets, Hasura console/dev-mode toggles, computed values (DATABASE_URL,
// DOCKER_NETWORK), and plugin env vars. All CLI docker compose invocations
// pass it via --env-file (see ComposeEnvFiles).
func WriteComposeEnv(workdir string, cfg *config.Config, secrets, pluginEnvVars map[string]string) error {
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
	for _, pw := range []struct{ key, val string }{
		{"POSTGRES_PASSWORD", cfg.Postgres.Password},
		{"REDIS_PASSWORD", cfg.Redis.Password},
	} {
		if enc := config.URLPassword(pw.val); enc != "" {
			merged[pw.key+config.URLPasswordEnvSuffix] = enc
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
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(composeEnvValue(k, merged[k]))
		sb.WriteString("\n")
	}

	path := filepath.Join(workdir, composeEnvFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s dir: %w", filepath.Dir(composeEnvFile), err)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", composeEnvFile, err)
	}
	// Enforce 0600 even when the file pre-existed with a looser mode.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", composeEnvFile, err)
	}
	return nil
}

// composeEnvValue renders one value for compose.env. An encoded password twin
// keeps "$" literal in a URL, and docker compose expands "$name" in an unquoted
// dotenv value, so a twin containing "$" is single-quoted (compose reads that
// verbatim; the encoder never emits a single quote). Everything else follows
// config.QuoteEnvValue.
func composeEnvValue(key, val string) string {
	if strings.HasSuffix(key, config.URLPasswordEnvSuffix) && strings.Contains(val, "$") {
		return "'" + val + "'"
	}
	return config.QuoteEnvValue(val)
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
	if len(files) == 1 && strings.HasSuffix(files[0], ".env") && !strings.HasSuffix(files[0], composeEnvFile) {
		return nil
	}
	return files
}
