package compose

// secret_literal.go — compose interpolation references for optional-service secrets.
//
// Purpose: keep secret values out of the generated docker-compose.yml at the
// source. Optional services emit ${VAR} references instead of literals; the
// value reaches the container through .nself/compose.env, which nself build
// writes from build.SecretEnvMap (plus the _URLENC password twins).
// Inputs: the env var name (a build.SecretEnvMap key) and the configured value.
// Outputs: "${NAME}" when set, "${NAME:-}" when unset, the literal when
// compose.env cannot carry the value unquoted (see dotenvSafe).
// Constraints: NAME must be a build.SecretEnvMap key, or compose.env never
// carries the value (secret_literal_external_test.go enforces this). D-0266.

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/nself-org/cli/internal/config"
)

// secretRef returns what to write for secret env var name in a generated
// compose file.
//
//   - Unset (empty or whitespace, the build.SecretEnvMap rule): "${NAME:-}".
//     Compose resolves it to the same empty string the old "" literal
//     carried, without a "variable is not set" warning, and nchat Hygiene
//     accepts it (it rejected `MEILI_MASTER_KEY: ""`).
//   - Set and dotenvSafe: "${NAME}". For a value of 8+ URL-safe characters
//     that is the exact text build.TemplateSecrets rewrote the literal to, so
//     those builds produce the same bytes as before. Shorter or URL-unsafe
//     values were left literal by the templater and now become references too.
//   - Set but not dotenvSafe: the literal, as before D-0266. compose.env is
//     written unquoted, so a reference would hand the container a truncated
//     or expanded value; the literal stays until compose.env quotes such
//     values (D-0329), and build.LiteralSecretLeaks still warns about it.
func secretRef(name, value string) string {
	switch {
	case strings.TrimSpace(value) == "":
		return "${" + name + ":-}"
	case !dotenvSafe(value):
		return value
	default:
		return "${" + name + "}"
	}
}

// dotenvSafe reports whether value survives an unquoted compose.env line
// intact. Docker Compose's dotenv parser trims surrounding whitespace, cuts
// at " #", expands "$" and treats quotes and backslashes specially.
func dotenvSafe(value string) bool {
	return !strings.ContainsFunc(value, unicode.IsSpace) && !strings.ContainsAny(value, "#$\"'`\\")
}

// secretURLUserInfo returns the "user:password" part of a connection URL with
// the password as a reference to its URL-encoded twin, falling back to the raw
// variable: ${NAME_URLENC:-${NAME}}. build.WriteComposeEnv writes the twin
// (config.URLPassword) for every set password, so the container gets the same
// encoded bytes config.URLUserInfo produced, and a password that needs
// encoding no longer lands in the file (the templater only rewrote URL-safe
// ones). An empty password keeps config.URLUserInfo's output unchanged.
func secretURLUserInfo(user, password, name string) string {
	if password == "" {
		return config.URLUserInfo(user, password)
	}
	return url.User(user).String() + ":${" + name + config.URLPasswordEnvSuffix + ":-${" + name + "}}"
}
