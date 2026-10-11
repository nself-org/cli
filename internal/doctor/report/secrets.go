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
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// MinSecretLen is the shortest value that is used as a literal scrub input.
const MinSecretLen = 6

// secretWords is the one definition of "secret-like name" (EPIC D21).
const secretWords = `SECRET|PASSWORD|PASS|TOKEN|KEY|PRIVATE|CREDENTIAL|DSN`

var secretNameRe = regexp.MustCompile(`(?i)` + secretWords)

// refValueRe matches a value that is only a compose/shell variable reference
// (${NAME} or $NAME): it carries no secret, so it is not a scrub input.
var refValueRe = regexp.MustCompile(`^(?:\$\{[A-Za-z_][A-Za-z0-9_]*\}|\$[A-Za-z_][A-Za-z0-9_]*)$`)

var hexEscapeRe = regexp.MustCompile(`%[0-9A-F]{2}`)

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
// a URL value with userinfo (any key, DATABASE_URL style); and, for a secret-like
// key holding a JSON object (HASURA_GRAPHQL_JWT_SECRET), each string field of it
// that is secret-like or named "key", so the inner key is scrubbed on its own.
func ExpandSecretValues(key, value string) []SecretValue {
	value = TrimQuotes(value)
	var out []SecretValue
	if sv, ok := NewSecretValue(key, value); ok {
		out = append(out, sv)
	}
	if u, err := url.Parse(value); err == nil && u.User != nil {
		if pw, has := u.User.Password(); has {
			if sv, ok := literalValue(key, pw); ok {
				out = append(out, sv)
			}
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

// variantsOf lists the raw value plus its URL-query, URL-path and JSON escaped
// forms (and lower-case-hex spellings of the URL forms) when they differ.
func variantsOf(raw string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if len(s) >= MinSecretLen && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	add(raw)
	for _, esc := range []string{url.QueryEscape(raw), url.PathEscape(raw)} {
		add(esc)
		add(hexEscapeRe.ReplaceAllStringFunc(esc, strings.ToLower))
	}
	for _, html := range []bool{true, false} {
		if s, ok := jsonEscape(raw, html); ok {
			add(s)
		}
	}
	if strings.ContainsAny(raw, "\r\n") { // multi-line value written on one line
		add(strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(raw))
	}
	return out
}

func jsonEscape(raw string, escapeHTML bool) (string, bool) {
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(escapeHTML)
	if enc.Encode(raw) != nil {
		return "", false
	}
	s := strings.TrimSpace(sb.String())
	if len(s) < 2 {
		return "", false
	}
	return s[1 : len(s)-1], true
}

type candidate struct{ text, key string }

// candidates flattens values into (spelling, key) pairs, longest first, so a
// value that is a substring of another can never leave the longer one's tail.
func candidates(values []SecretValue) []candidate {
	var out []candidate
	for _, v := range values {
		for _, s := range v.Variants {
			out = append(out, candidate{s, v.Key})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].text) > len(out[j].text) })
	return out
}
