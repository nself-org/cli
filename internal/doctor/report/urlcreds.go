package report

// urlcreds.go: learn the password of a URL-shaped value (M5).
//
// Purpose: a password with a raw #, ?, / or space makes url.Parse fail or
// split the URL wrongly, so the strict parse learns nothing; this reads the
// value leniently so the password still becomes a scrub input.
// Inputs: a key and its (unquoted) value.
// Outputs: user/password pairs.
// Constraints: pure. The userinfo ends at the LAST @ of the value; the user
// part may not hold / ? # @ or space. Schemeless user:pass@host is accepted
// only for secret-named or *_URL / *_URI keys.

import (
	"net/url"
	"regexp"
	"strings"
)

type urlCred struct{ user, pass string }

var (
	schemePrefixRe = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z0-9+.\-]*:)?//`)
	urlNameRe      = regexp.MustCompile(`(?i)(?:URL|URI)$`)
	hostStartRe    = regexp.MustCompile(`^[A-Za-z0-9_.\[(\-]`)
)

func urlCredentials(key, value string) []urlCred {
	var out []urlCred
	if u, err := url.Parse(value); err == nil && u.User != nil {
		if pw, has := u.User.Password(); has {
			out = append(out, urlCred{u.User.Username(), pw})
		}
	}
	rest, schemed := value, false
	if m := schemePrefixRe.FindString(value); m != "" {
		rest, schemed = value[len(m):], true
	} else if !IsSecretName(key) && !urlNameRe.MatchString(key) {
		return out
	}
	at := strings.LastIndex(rest, "@")
	if at < 1 || !hostStartRe.MatchString(rest[at+1:]) {
		return out
	}
	user, pass, ok := strings.Cut(rest[:at], ":")
	if !ok || strings.ContainsAny(user, "/?#@ \t\r\n") || (user == "" && !schemed) {
		return out
	}
	return append(out, urlCred{user, pass})
}
