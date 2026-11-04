package entitlement

// Multi-token publication for contract plugin.entitlement-token v1.1 (D-0851).
// Purpose: let a customer with two bundles under two licence keys run both.
// Inputs: verified candidate JWTs, installed licensed slugs, rejected subjects.
// Outputs: ordered lines, one JWT per accepted configured key.
// Constraints: line 1 is the token covering the most installed plugins, so a
// single-line read gets the best single answer; when line 1 alone covers every
// installed licensed plugin the file is exactly that token (byte-identical v1).
// SPORT: plugin.entitlement-token v1.1.

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/license"
)

// rankedToken is one verified candidate with its ordering facts.
type rankedToken struct {
	token   string
	sub     string
	exp     int64
	inGrace bool
	covered int
}

// Purpose: rank a candidate. Inputs: token/claims/slugs. Outputs: rank facts.
// Constraints: coverage counts only installed licensed slugs. SPORT: v1.1 ordering.
func rankToken(token string, claims *license.LicenseClaims, names []string) rankedToken {
	allowed := make(map[string]bool, len(claims.Plugins))
	for _, name := range claims.Plugins {
		allowed[name] = true
	}
	r := rankedToken{token: token, sub: claims.Sub, exp: claims.Exp,
		inGrace: !time.Now().After(time.Unix(claims.Exp, 0).Add(license.PostExpiryGraceWindow))}
	for _, name := range names {
		if allowed["*"] || allowed[name] {
			r.covered++
		}
	}
	return r
}

// Purpose: order two candidates. Inputs: ranks. Outputs: a before b.
// Constraints: in-grace, then most installed covered, then newest signed exp.
// SPORT: v1.1 ordering.
func (a rankedToken) better(b rankedToken) bool {
	if a.inGrace != b.inGrace {
		return a.inGrace
	}
	if c := cmp.Compare(a.covered, b.covered); c != 0 {
		return c == 1
	}
	return a.exp > b.exp // equal exp is not better: ties keep the first seen
}

// Purpose: select one token per accepted configured key and order the lines.
// Inputs: candidates/slugs/rejected. Outputs: lines/policy error naming each
// uncovered slug. Constraints: rejected, persisted-rejected, revoked and
// unconfigured subjects grant nothing; past-grace tokens are written only as the
// sole diagnostic line when nothing is in grace. SPORT: plugin.entitlement-token v1.1.
func selectTokens(candidates, names []string, rejected map[string]bool) ([]string, error) {
	configured := make(map[string]bool)
	for _, key := range license.CollectLicenseKeys() {
		configured[license.HashKey(key)] = true
	}
	persisted, err := loadRejected()
	if err != nil {
		return nil, fmt.Errorf("the persisted licence rejection record is unreadable: %w", err)
	}
	perKey := make(map[string]rankedToken)
	var order []string
	for _, token := range candidates {
		claims, err := license.ParseLicenseJWT(token)
		if err != nil || claims.Exp <= 0 || !configured[claims.Sub] || rejected[claims.Sub] || persisted[claims.Sub] ||
			license.IsRecordRevoked(license.LicenseRecord{KeyHash: claims.Sub}) {
			continue
		}
		r := rankToken(token, claims, names)
		prev, seen := perKey[r.sub]
		if !seen {
			order = append(order, r.sub)
		}
		if !seen || r.better(prev) {
			perKey[r.sub] = r
		}
	}
	ranked := make([]rankedToken, 0, len(order))
	for _, sub := range order {
		ranked = append(ranked, perKey[sub])
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].better(ranked[j]) || ranked[j].better(ranked[i]) {
			return ranked[i].better(ranked[j])
		}
		return ranked[i].sub < ranked[j].sub
	})
	if len(ranked) == 0 {
		return nil, fmt.Errorf("no verified entitlement belongs to an accepted configured licence")
	}
	if !ranked[0].inGrace {
		newest := ranked[0]
		for _, r := range ranked[1:] {
			if r.exp > newest.exp {
				newest = r
			}
		}
		return []string{newest.token}, tokenPolicy(newest.token, names)
	}
	var lines []string
	for _, r := range ranked {
		if r.inGrace {
			lines = append(lines, r.token)
		}
	}
	if ranked[0].covered == len(names) {
		return lines[:1], nil
	}
	return lines, uncoveredError(lines, names)
}

// Purpose: name every installed licensed slug no line covers (E125 text).
// Inputs: lines/slugs. Outputs: nil or the uncovered list. SPORT: v1.1 E125.
func uncoveredError(lines, names []string) error {
	allowed := make(map[string]bool)
	for _, token := range lines {
		if claims, err := license.ParseLicenseJWT(token); err == nil {
			for _, name := range claims.Plugins {
				allowed[name] = true
			}
		}
	}
	var missing []string
	for _, name := range names {
		if !allowed["*"] && !allowed[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("entitlement does not cover installed plugins: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Purpose: retain verified runtime lines if the cache holds another key or is absent.
// Inputs: file path. Outputs: tokens. Constraints: each line needs a configured
// subject and an unexpired signed grace; bounded regular-file read, no symlinks.
// SPORT: reboot/offline continuity, one line per key (v1.1).
func retainedTokens(path string) []string {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	f, err := os.OpenFile(path, os.O_RDONLY|noFollowFlag, 0)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return nil
	}
	configured := make(map[string]bool)
	for _, key := range license.CollectLicenseKeys() {
		configured[license.HashKey(key)] = true
	}
	var tokens []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		claims, err := license.ParseLicenseJWT(line)
		if err != nil || claims.Exp <= 0 || time.Now().After(time.Unix(claims.Exp, 0).Add(license.PostExpiryGraceWindow)) {
			continue
		}
		if configured[claims.Sub] {
			tokens = append(tokens, line)
		}
	}
	return tokens
}

// Purpose: keep other keys' lines through a scheduled refresh. Inputs: renewed
// tokens and file path. Outputs: renewed plus retained lines of subjects that
// were not renewed. Constraints: a renewed key never falls back to its older
// line, so a narrowed bundle is never widened. SPORT: plugin.entitlement-token v1.1.
func withRetained(renewed []string, path string) []string {
	fresh := make(map[string]bool)
	for _, token := range renewed {
		if claims, err := license.ParseLicenseJWT(token); err == nil {
			fresh[claims.Sub] = true
		}
	}
	out := append([]string(nil), renewed...)
	for _, token := range retainedTokens(path) {
		if claims, err := license.ParseLicenseJWT(token); err == nil && !fresh[claims.Sub] {
			out = append(out, token)
		}
	}
	return out
}
