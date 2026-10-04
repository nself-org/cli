// Package main — allowlist.go
// Known-safe auth middleware names and exempt route patterns.
// Adding an entry to either list requires a PR with documented rationale.
package main

// AllowedAuthMiddleware maps middleware function name to its source package.
// Any route whose middleware chain contains at least one of these passes the gate.
var AllowedAuthMiddleware = map[string]string{
	"RequireLicenseKey":     "plugins/shared/go/auth", // existing license middleware
	"RequireHasuraAdminKey": "plugins/shared/go/auth",
	"RequireUserJWT":        "plugins/shared/go/auth",
	"AdminOnly":             "admin/pkg/middleware",
	"InternalNetworkOnly":   "web/backend/pkg/middleware", // IP allowlist for internal-only routes
}

// ExemptRoute describes an HTTP path pattern that is explicitly allowed to have no auth,
// with a documented reason.
type ExemptRoute struct {
	// Pattern is matched as an exact path or prefix (if it ends with /*).
	Pattern string
	// Reason is required — explains why no auth is needed.
	Reason string
}

// ExemptRoutes lists routes that must not trigger auth-gate violations.
// Every entry requires a documented reason.
var ExemptRoutes = []ExemptRoute{
	{
		Pattern: "/health",
		Reason:  "public health check — no data exposure",
	},
	{
		Pattern: "/metrics",
		Reason:  "prometheus scrape — network-restricted by nginx",
	},
}

// isExempt reports whether path is covered by any ExemptRoute.
func isExempt(path string) bool {
	for _, e := range ExemptRoutes {
		pattern := e.Pattern
		if pattern == path {
			return true
		}
		// Prefix match: pattern ending in /* matches any sub-path.
		if len(pattern) > 2 && pattern[len(pattern)-2:] == "/*" {
			prefix := pattern[:len(pattern)-2]
			if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
				return true
			}
		}
		// chi-style parameter pattern: treat {param} as wildcard segment.
		if matchChiPattern(pattern, path) {
			return true
		}
	}
	return false
}

// matchChiPattern does a simple segment-by-segment match where {xxx} matches any single segment.
func matchChiPattern(pattern, path string) bool {
	ps := splitPath(pattern)
	vs := splitPath(path)
	if len(ps) != len(vs) {
		return false
	}
	for i, seg := range ps {
		if seg == vs[i] {
			continue
		}
		if len(seg) > 2 && seg[0] == '{' && seg[len(seg)-1] == '}' {
			continue // wildcard segment
		}
		return false
	}
	return true
}

func splitPath(p string) []string {
	var parts []string
	cur := ""
	for _, c := range p {
		if c == '/' {
			if cur != "" {
				parts = append(parts, cur)
				cur = ""
			}
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	return parts
}
