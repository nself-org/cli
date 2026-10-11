package compose

// secret_literal.go — compose interpolation references for optional-service secrets.
//
// Purpose: keep secret values out of the generated docker-compose.yml at the
// source. Optional services emit ${VAR} references instead of literals; the
// value reaches the container through .nself/compose.env, which nself build
// writes from build.SecretEnvMap.
// Inputs: the env var name (a build.SecretEnvMap key) and the configured value.
// Outputs: "${NAME}" when the value is set, "${NAME:-}" when it is not.
// Constraints: NAME must be a build.SecretEnvMap key, or compose.env never
// carries the value (secret_literal_external_test.go enforces this). D-0266.

import "strings"

// secretRef returns the compose interpolation reference for secret env var
// name. A set value yields "${NAME}", the exact text build.TemplateSecrets
// rewrites a literal to, so a build with the secret set produces the same
// bytes as before. An unset value yields "${NAME:-}": compose resolves it to
// the same empty string the old "" literal carried, without a "variable is
// not set" warning, and no literal lands in a generated file that may be
// committed (nchat Hygiene rejects `MEILI_MASTER_KEY: ""`). "Set" matches
// build.SecretEnvMap, which omits whitespace-only values.
func secretRef(name, value string) string {
	if strings.TrimSpace(value) == "" {
		return "${" + name + ":-}"
	}
	return "${" + name + "}"
}
