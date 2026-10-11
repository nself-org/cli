package report

// secrets.go: which names are secret-like, which values become scrub inputs,
// and the encodings a value may appear in inside logs and generated config.
//
// Purpose: turn env cascade entries and compose environment entries into the
// list of literal strings the scrubber must never let through.
// Inputs: key/value pairs (already read from the env cascade or compose).
// Outputs: SecretValue entries (key + every spelling of the value).
// Constraints: pure functions, no I/O. A value shorter than MinSecretLen is
// ignored (too short to match without wrecking the output).

import (
	"encoding/json"
	"regexp"
	"strings"
)

// MinSecretLen is the shortest value that is used as a literal scrub input.
const MinSecretLen = 6

// secretWords is the one definition of "secret-like name" (EPIC D21 as widened
// by the lead ruling R2). A name ending in _AUTH also counts (secretAuthSuffix);
// a bare AUTH prefix (AUTH_URL) does not.
const secretWords = `SECRET|PASSWORD|PASS|TOKEN|KEY|PRIVATE|CREDENTIAL|DSN|SALT|SEED|LICEN[CS]E|COOKIE|SESSION|SIGNING|HTPASSWD|BASIC[_-]?AUTH`

// secretIdent matches a whole secret-like identifier inside free text; the
// assignment pass builds on it so both passes share one definition.
const secretIdent = `(?:[A-Za-z0-9_.\-]*(?:` + secretWords + `)[A-Za-z0-9_.\-]*|[A-Za-z0-9_.\-]*[_-]AUTH)`

var secretNameRe = regexp.MustCompile(`(?i)(?:` + secretWords + `)|_AUTH$`)

// refValueRe matches a value that is only a compose/shell variable reference
// (${NAME} or $NAME): it carries no secret, so it is not a scrub input.
var refValueRe = regexp.MustCompile(`^(?:\$\{[A-Za-z_][A-Za-z0-9_]*\}|\$[A-Za-z_][A-Za-z0-9_]*)$`)

// IsSecretName reports whether an env/compose key name looks secret-bearing.
func IsSecretName(name string) bool { return secretNameRe.MatchString(name) }

// SecretValue is one literal to remove, in every spelling it may take.
type SecretValue struct {
	Key      string
	Variants []string
}

// TrimQuotes removes one matching pair of surrounding quotes.
func TrimQuotes(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// NewSecretValue returns the scrub input for key=value, or false when the key
// is not secret-like, the value is too short, or it is only a ${REF}.
func NewSecretValue(key, value string) (SecretValue, bool) {
	value = TrimQuotes(value)
	if !IsSecretName(key) || len(value) < MinSecretLen || refValueRe.MatchString(value) {
		return SecretValue{}, false
	}
	return SecretValue{Key: key, Variants: variantsOf(value)}, true
}

// ExpandSecretValues returns every literal the scrubber should learn from one
// key=value pair: the value itself when the key is secret-like; the password of
// a URL-shaped value (any key, DATABASE_URL style, even when url.Parse refuses
// it: see urlcreds.go); and, for a secret-like key holding a JSON object
// (HASURA_GRAPHQL_JWT_SECRET), each string field of it that is secret-like or
// named "key", so the inner key is scrubbed on its own.
func ExpandSecretValues(key, value string) []SecretValue {
	value = TrimQuotes(value)
	var out []SecretValue
	if sv, ok := NewSecretValue(key, value); ok {
		out = append(out, sv)
	}
	for _, c := range urlCredentials(key, value) {
		if sv, ok := literalValue(key, c.pass); ok {
			sv.Variants = append(sv.Variants, basicAuthVariants(c.user+":"+c.pass)...)
			out = append(out, sv)
		}
	}
	var obj map[string]any
	if IsSecretName(key) && strings.HasPrefix(value, "{") && json.Unmarshal([]byte(value), &obj) == nil {
		for k, v := range obj {
			if str, ok := v.(string); ok && (IsSecretName(k) || strings.EqualFold(k, "key")) {
				if sv, ok := literalValue(key, str); ok {
					out = append(out, sv)
				}
			}
		}
	}
	return out
}

// literalValue is a scrub input with no name check (the caller decided).
func literalValue(key, value string) (SecretValue, bool) {
	if len(value) < MinSecretLen || refValueRe.MatchString(value) {
		return SecretValue{}, false
	}
	return SecretValue{Key: key, Variants: variantsOf(value)}, true
}
