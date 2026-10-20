package plugin

import (
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
)

func TestPinnedBinaryMissingSignatureRefused(t *testing.T) {
	for _, paid := range []bool{false, true} {
		for _, bypass := range []bool{false, true} {
			t.Run(releaseTier(paid)+map[bool]string{false: "/normal", true: "/bypass"}[bypass], func(t *testing.T) {
				key, _ := signaturetest.NewKey(t, releaseTier(paid))
				signaturetest.UseKeys(t, []signature.Key{key}, nil)
				m := &PluginManifest{Name: "fixture", Version: "1.0.0"}
				if got := codeOf(verifyReleaseSignature(m, paid, false, "linux-amd64", "", bypass)); got != "E121" {
					t.Fatalf("missing pinned binary signature: code %q, want E121", got)
				}
			})
		}
	}
}
