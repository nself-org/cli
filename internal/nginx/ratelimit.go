package nginx

import "strings"

// generateRateLimits renders the rate-limits.conf from the embedded template.
//
// This produces all 10 rate limiting zones defined in BUILD_SPEC Part 12:
//
//	Request rate zones (limit_req_zone):
//	  general       — 10r/s   keyed on $binary_remote_addr
//	  graphql_api   — 100r/m  keyed on $binary_remote_addr
//	  auth          — 30r/m   keyed on $binary_remote_addr (configurable via AUTH_RATE_LIMIT)
//	  uploads       — 5r/m    keyed on $binary_remote_addr
//	  user_api      — 1000r/m keyed on $http_authorization
//	  static        — 1000r/m keyed on $binary_remote_addr
//	  webhooks      — 30r/m   keyed on $binary_remote_addr
//	  functions     — 50r/m   keyed on $binary_remote_addr
//
//	Connection zones (limit_conn_zone):
//	  conn_limit_per_ip  — keyed on $binary_remote_addr
//	  conn_limit_server  — keyed on $server_name
//
//	Status codes: limit_req_status 429, limit_conn_status 429
func (g *Generator) generateRateLimits() (string, error) {
	if err := g.ensureModel(); err != nil {
		return "", err
	}
	rate := func(name string) string {
		for _, z := range g.model.Zones {
			if z.Name == name && z.Rate != nil {
				return *z.Rate
			}
		}
		return ""
	}
	data := map[string]string{
		"AuthRateLimit": rate("auth"),
		"RateLimitAPI":  strings.TrimSuffix(rate("api"), "r/s"),
		"RateLimitAuth": strings.TrimSuffix(rate("auth_strict"), "r/s"),
		"RateLimitAI":   strings.TrimSuffix(rate("ai"), "r/s"),
	}
	return g.render("rate-limits.conf.tmpl", data)
}
