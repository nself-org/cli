package doctor

// URL-reserved password advisory (P7-PROD-31).
//
// Purpose: docker compose substitutes a variable into a URL verbatim. A
// password with "/", "@", ":", "?", "#" or "%" breaks any plugin compose
// fragment that writes ":${POSTGRES_PASSWORD}@" instead of the encoded twin
// ${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}. This finds that
// combination so doctor can warn.
//
// Inputs:  the project's resolved passwords and the text of each installed
//          plugin compose fragment.
// Outputs: the NAMES of the password variables still embedded raw in a URL.
//          Never a value: callers print names only.
// Constraints: advisory, never a failure; pure functions, no I/O.

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// PasswordNeedsURLEncoding reports whether pw changes under URL userinfo
// encoding, i.e. whether embedding it raw in a URL would break the URL.
func PasswordNeedsURLEncoding(pw string) bool {
	return pw != "" && config.URLPassword(pw) != pw
}

// rawRefPattern matches a reference to name without a URLENC twin in front of
// it: ${NAME}, ${NAME:-...} or $NAME.
func rawRefPattern(name string) *regexp.Regexp {
	q := regexp.QuoteMeta(name)
	return regexp.MustCompile(`\$\{` + q + `[}:\-]|\$` + q + `\b`)
}

// RawPasswordInURL reports whether fragment embeds ${name} raw inside a URL
// (a line holding "://" with the variable after it). The safe nested form
// ${NAME_URLENC:-${NAME}} is not raw use and is ignored; so are comments.
func RawPasswordInURL(fragment, name string) bool {
	safe := "${" + name + config.URLPasswordEnvSuffix + ":-${" + name + "}}"
	re := rawRefPattern(name)
	for _, line := range strings.Split(fragment, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "#") {
			continue
		}
		i := strings.Index(line, "://")
		if i < 0 {
			continue
		}
		rest := strings.ReplaceAll(line[i:], safe, "")
		if re.MatchString(rest) {
			return true
		}
	}
	return false
}

// URLReservedFindings returns, sorted, the names of the passwords (keys of
// passwords) that need URL encoding and are still embedded raw in a URL in at
// least one of fragments. Values of passwords are read, never returned.
func URLReservedFindings(passwords map[string]string, fragments []string) []string {
	var out []string
	for name, pw := range passwords {
		if !PasswordNeedsURLEncoding(pw) {
			continue
		}
		for _, f := range fragments {
			if RawPasswordInURL(f, name) {
				out = append(out, name)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// URLReservedMessages reads the fragment files at paths (unreadable files are
// skipped: advisory, never an error) and returns one warning per password
// variable found raw in a URL. A message names the variable and the fix, never
// the password.
func URLReservedMessages(paths []string, passwords map[string]string) []string {
	var fragments []string
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			fragments = append(fragments, string(data))
		}
	}
	var msgs []string
	for _, name := range URLReservedFindings(passwords, fragments) {
		msgs = append(msgs, fmt.Sprintf(
			"%s contains URL-reserved characters and an installed plugin fragment embeds it raw in a URL; use ${%s%s:-${%s}} there (see Plugin-Dev-Guide)",
			name, name, config.URLPasswordEnvSuffix, name))
	}
	return msgs
}
