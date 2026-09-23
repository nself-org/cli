package config

import "net/url"

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
