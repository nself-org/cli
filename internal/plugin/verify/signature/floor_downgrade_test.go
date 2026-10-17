package signature_test

import (
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
)

func TestUnsignedFloorDowngradeRegression(t *testing.T) {
	signaturetest.UseKeys(t, nil, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{
		signature.TierFree: {"ci": "1.2.7"}, signature.TierLicensed: {},
	})
	for _, v15 := range []bool{false, true} {
		for _, entry := range []struct{ tier, version string }{
			{signature.TierFree, "0.0.1"},
			{signature.TierFree, "1.2.7-rc.1"},
			{signature.TierLicensed, "1.2.7"},
			{signature.TierLicensed, "0.0.1"},
		} {
			_, err := signature.Decide(signature.Input{
				Tier: entry.tier, Slug: "ci", Version: entry.version,
				SHA256: digest("forged archive"), V15: v15,
			})
			if code(err) != "E107" {
				t.Errorf("tier=%s version=%s v15=%v: want E107, got %v", entry.tier, entry.version, v15, err)
			}
		}
	}
}
