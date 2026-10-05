package config

import (
	"net/url"
	"strings"
)

// URLUserInfo returns the "user:password" part of a connection URL with both
// halves percent-encoded for the userinfo component (RFC 3986 §3.2.1).
//
// Purpose: every DSN the CLI builds (DATABASE_URL, REDIS_URL, pgbouncer and
// restore URLs) embeds a credential. A password containing "/", "@", ":", "?"
// or "#" must be escaped there, or the URL parses with the wrong host and
// clients fail with "Invalid URL" (nself-web prod, 2026-09-23: a base64
// REDIS_PASSWORD with "/" left auth-server rate limiting, revocation and
// lockout without Redis). url.PathEscape is not a substitute: it leaves "@"
// and ":" unescaped.
// Inputs: user may be empty (Redis uses ":password"); password may be empty.
// Outputs: "user", "user:pw" or ":pw", escaped; "" when both are empty.
func URLUserInfo(user, password string) string {
	if password == "" {
		if user == "" {
			return ""
		}
		return url.User(user).String()
	}
	return url.UserPassword(user, password).String()
}

// URLPasswordEnvSuffix is appended to a password variable name to name its
// URL-encoded twin (POSTGRES_PASSWORD -> POSTGRES_PASSWORD_URLENC).
const URLPasswordEnvSuffix = "_URLENC"

// URLPassword returns password percent-encoded for the password position of a
// connection URL's userinfo (RFC 3986 §3.2.1): the exact bytes URLUserInfo
// emits after the colon, so "redis://:" + URLPassword(p) + "@host" and
// URLUserInfo("", p) agree.
//
// Purpose: docker compose can interpolate a variable into a URL but cannot
// encode it, so a password with "/", "@", ":", "?", "#" or "%" breaks every
// compose fragment that writes ":${POSTGRES_PASSWORD}@" (P7-PROD-31). nself
// build writes this value as POSTGRES_PASSWORD_URLENC / REDIS_PASSWORD_URLENC
// and fragments reference it. Encode once, at the source: the result must be
// substituted verbatim and never encoded again (a second pass turns "%40" into
// "%2540").
// Inputs: any password, possibly empty.
// Outputs: the encoded password, or "" for an empty password (callers omit the
// variable so the compose fallback applies).
func URLPassword(password string) string {
	if password == "" {
		return ""
	}
	// "x" is a placeholder username; only the password half is returned.
	return strings.TrimPrefix(url.UserPassword("x", password).String(), "x:")
}
