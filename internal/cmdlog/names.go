package cmdlog

// Purpose: flag-name and URL rules for the command log (D-0473).
// Inputs: one argv element and the element after it.
// Outputs: whether a flag name carries a secret value; an element with URL
// credentials replaced.
// Constraints: stdlib only. internal/config imports packages that would
// cycle back here, so the word list is a copy of config.secretKeyParts
// (internal/config/secretkeys.go) plus AUTH; keep the two in step. Matching
// over-masks (--keyboard hides its value); that is the safe side.

import (
	"regexp"
	"strings"
)

// secretNameParts mirrors config.secretKeyParts (SECRET, PASS covering
// password/passwd, KEY covering apikey/api-key/api_key/private-key, TOKEN,
// CREDENTIAL, WEBHOOK, PRIVATE, DSN, SALT) and adds AUTH.
var secretNameParts = []string{"SECRET", "PASS", "KEY", "TOKEN", "CREDENTIAL", "WEBHOOK", "PRIVATE", "DSN", "SALT", "AUTH"}

// secretFlagName reports whether a long flag name (with its leading dashes and
// no "=value") names a secret. Dashes and underscores are ignored.
func secretFlagName(name string) bool {
	if !strings.HasPrefix(name, "--") {
		return false
	}
	flat := strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(name))
	for _, part := range secretNameParts {
		if strings.Contains(flat, part) {
			return true
		}
	}
	return false
}

// urlCredentials matches scheme://user:password@ up to the last "@" of the
// whitespace-delimited chunk, so a password holding "@" or "/" cannot leak.
var urlCredentials = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://[^\s:/@]+):\S*@`)

// maskURLCredentials replaces the password of every URL user:password@ in s.
func maskURLCredentials(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	return urlCredentials.ReplaceAllString(s, "${1}:"+redactedText+"@")
}
