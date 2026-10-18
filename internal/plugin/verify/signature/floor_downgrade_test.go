package signature_test

import (
	"strings"
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

// TestPresigningAllowlist: with the floor naming a slug, only the listed
// (tier, slug, version, sha256) pairs may go unsigned, and the lookup is per
// tier pair so a slug released in both tiers keeps its two histories apart.
func TestPresigningAllowlist(t *testing.T) {
	signaturetest.UseKeys(t, nil, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{
		signature.TierFree: {"cron": "1.2.0"}, signature.TierLicensed: {"cron": "3.0.0"},
	})
	sha := digest("old archive")
	signaturetest.UsePresigning(t, map[string]map[string][]signature.PresigningRelease{
		signature.TierFree:     {"cron": {{Version: "1.0.0", SHA256: sha}}},
		signature.TierLicensed: {"cron": {{Version: "2.0.0", SHA256: digest("pro archive")}}},
	})
	cases := []struct {
		name, tier, version, sha string
		ok                       bool
	}{
		{"listed free release", signature.TierFree, "1.0.0", sha, true},
		{"listed free release, uppercase digest", signature.TierFree, "1.0.0", strings.ToUpper(sha), true},
		{"same version, other bytes", signature.TierFree, "1.0.0", digest("forged"), false},
		{"free version listed only for licensed", signature.TierFree, "2.0.0", digest("pro archive"), false},
		{"licensed release listed for licensed", signature.TierLicensed, "2.0.0", digest("pro archive"), true},
		{"free pair presented as licensed", signature.TierLicensed, "1.0.0", sha, false},
		{"empty digest", signature.TierFree, "1.0.0", "", false},
	}
	for _, c := range cases {
		for _, v15 := range []bool{false, true} {
			_, err := signature.Decide(signature.Input{Tier: c.tier, Slug: "cron", Version: c.version, SHA256: c.sha, V15: v15})
			wantOK := c.ok && !v15 // a listed release still gets the ordinary rule: warn in v1.4, refuse in v1.5
			if (err == nil) != wantOK || (err != nil && code(err) != "E107") {
				t.Errorf("%s v15=%v: err=%v, want ok=%v", c.name, v15, err, wantOK)
			}
		}
	}
}
