package compose

// secret_literal.go — compose interpolation references for optional-service secrets.
//
// Purpose: keep secret values out of the generated docker-compose.yml at the
// source. Optional services emit ${VAR} references instead of literals; the
// value reaches the container through .nself/compose.env, which nself build
// writes from build.SecretEnvMap (plus the _URLENC password twins).
// Inputs: the env var name (a build.SecretEnvMap key) and the configured value.
// Outputs: "${NAME}" when set, "${NAME:-}" when unset.
// Constraints: NAME must be a build.SecretEnvMap key, or compose.env never
// carries the value (secret_literal_external_test.go enforces this). D-0266.

import (
	"net/url"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// secretRef returns what to write for secret env var name in a generated
// compose file.
//
//   - Unset (empty or whitespace, the build.SecretEnvMap rule): "${NAME:-}".
//     Compose resolves it to the same empty string the old "" literal
//     carried, without a "variable is not set" warning, and nchat Hygiene
//     accepts it (it rejected `MEILI_MASTER_KEY: ""`).
//   - Set: "${NAME}". For a value of 8+ URL-safe characters that is the
//     exact text build.TemplateSecrets rewrote the literal to, so those
//     builds produce the same bytes as before. Values of any other shape
//     (short, URL-unsafe, holding whitespace, '#', '$', quotes or
//     backslashes) become references too: compose.env quotes each value so
//     compose reads it back verbatim (config.QuoteComposeEnvValue, D-0333).
func secretRef(name, value string) string {
	if strings.TrimSpace(value) == "" {
		return "${" + name + ":-}"
	}
	return "${" + name + "}"
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
