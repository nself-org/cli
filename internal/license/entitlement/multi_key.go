package entitlement

import (
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/license"
)

// Purpose: gather independently verified per-key cache tokens.
// Inputs: configured keys. Outputs: candidates. Constraints: signed body and JWT
// must agree on the configured subject. SPORT: plugin.entitlement-token v1.1.
func cachedTokens() []string {
	var tokens []string
	for _, key := range license.CollectLicenseKeys() {
		entry, err := license.ReadCacheForKey(key)
		if err != nil || entry == nil || !entry.VerifySignature() {
			continue
		}
		claims, err := license.ParseLicenseJWT(entry.JWT)
		if err == nil && claims.Exp > 0 && claims.Sub == license.HashKey(key) {
			tokens = append(tokens, entry.JWT)
		}
	}
	return tokens
}

// Purpose: publish one valid token per key with union coverage.
// Inputs: candidates, installed slugs, rejected hashes. Outputs: lines/error.
// Constraints: covering token first for legacy readers, then signed expiry;
// single-token diagnostics are preserved. SPORT: plugin.entitlement-token v1.1.
func selectTokens(candidates, names []string, rejected map[string]bool) (string, error) {
	groups := make(map[string][]string)
	for _, candidate := range candidates {
		for _, token := range strings.Split(candidate, "\n") {
			token = strings.TrimSpace(token)
			claims, err := license.ParseLicenseJWT(token)
			if err == nil {
				groups[claims.Sub] = append(groups[claims.Sub], token)
			}
		}
	}
	var tokens []string
	for _, group := range groups {
		var valid []string
		for _, token := range group {
			if tokenPolicy(token, nil) == nil {
				valid = append(valid, token)
			}
		}
		token, _ := selectToken(valid, names, rejected)
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	if len(tokens) == 0 {
		return selectToken(candidates, names, rejected)
	}
	sort.Slice(tokens, func(i, j int) bool {
		iCovers, jCovers := tokenPolicy(tokens[i], names) == nil, tokenPolicy(tokens[j], names) == nil
		if iCovers != jCovers {
			return iCovers
		}
		a, _ := license.ParseLicenseJWT(tokens[i])
		b, _ := license.ParseLicenseJWT(tokens[j])
		if a.Exp != b.Exp {
			return a.Exp > b.Exp
		}
		return a.Sub < b.Sub
	})
	result := strings.Join(tokens, "\n")
	return result, tokenPolicy(result, names)
}
