package entitlement

import (
	"fmt"

	"github.com/nself-org/cli/internal/license"
)

// Purpose: select the newest signed covering token among configured subjects.
// Inputs: candidates/slugs/rejected subjects. Outputs: token/policy error.
// Constraints: rejected keys (this run, persisted, or on the revocation list) grant
// no authority; signed exp orders candidates.
// SPORT: single-token multi-key continuity.
func selectToken(candidates []string, names []string, rejected map[string]bool) (string, error) {
	best, fallback := "", ""
	var bestExp, fallbackExp int64
	configured := make(map[string]bool)
	for _, key := range license.CollectLicenseKeys() {
		configured[license.HashKey(key)] = true
	}
	persisted := loadRejected()
	for _, token := range candidates {
		claims, err := license.ParseLicenseJWT(token)
		if err != nil || claims.Exp <= 0 || !configured[claims.Sub] || rejected[claims.Sub] || persisted[claims.Sub] ||
			license.IsRecordRevoked(license.LicenseRecord{KeyHash: claims.Sub}) {
			continue
		}
		if claims.Exp > fallbackExp {
			fallback, fallbackExp = token, claims.Exp
		}
		if tokenPolicy(token, names) == nil && claims.Exp > bestExp {
			best, bestExp = token, claims.Exp
		}
	}
	if best != "" {
		return best, nil
	}
	if fallback != "" {
		return fallback, tokenPolicy(fallback, names)
	}
	return "", fmt.Errorf("no verified entitlement belongs to an accepted configured licence")
}

// Purpose: reject only the newly refused key when another key still covers.
// Inputs: project/key. Outputs: updated file/error. Constraints: no online retry.
// SPORT: multi-key license-set revocation isolation.
func publishAfterKeyRejection(projectDir, key string) error {
	names, err := installedLicensedNames("")
	if err != nil {
		return err
	}
	token, _ := cachedToken()
	return publishCandidates(projectDir, names, map[string]bool{license.HashKey(key): true},
		[]string{token, retainedToken(Path(projectDir))})
}
