package report

// variants.go: every spelling a secret value may take in logs and config.
//
// Purpose: the literal pass can only remove what it can spell, so each value
// is expanded: URL-query / URL-path / JSON escapes, lowercase-hex escapes,
// base64 (standard and URL alphabets, padded and raw), lowercase hex, the
// shell single-quote form, and each line of a multi-line value. For URL
// credentials, base64 of user:pass (HTTP Basic) as well.
// Inputs: a raw value.
// Outputs: distinct spellings of length >= MinSecretLen.
// Constraints: pure. Base64 inside a larger base64 blob depends on alignment
// and is best-effort (documented in the wiki page).

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var hexEscapeRe = regexp.MustCompile(`%[0-9A-F]{2}`)

// variantsOf lists the raw value plus its encoded forms when they differ.
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
		for _, line := range strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == '\r' }) {
			add(strings.TrimSpace(line)) // each line is its own candidate (M2)
		}
	}
	for _, s := range encodedForms(raw) {
		add(s)
	}
	if strings.Contains(raw, "'") {
		add(strings.ReplaceAll(raw, "'", `'\''`))
	}
	return out
}

// encodedForms are the base64 spellings and the lowercase hex of s.
func encodedForms(s string) []string {
	b := []byte(s)
	return []string{
		base64.StdEncoding.EncodeToString(b), base64.RawStdEncoding.EncodeToString(b),
		base64.URLEncoding.EncodeToString(b), base64.RawURLEncoding.EncodeToString(b),
		hex.EncodeToString(b),
	}
}

// basicAuthVariants are the encodings of a "user:pass" pair (HTTP Basic).
func basicAuthVariants(pair string) []string {
	var out []string
	for _, s := range encodedForms(pair) {
		if len(s) >= MinSecretLen {
			out = append(out, s)
		}
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

// candidates flattens values into (spelling, key) pairs, longest first.
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
