package plugin

import (
	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
	"testing"
)

func TestSignedCLISourceFallbackRefused(t *testing.T) {
	for _, binary := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-binary", true: "with-binary"}[binary], func(t *testing.T) {
			key, signer := signaturetest.NewKey(t, signature.TierFree)
			signaturetest.UseKeys(t, []signature.Key{key}, nil)
			signaturetest.UseFloor(t, map[string]map[string]string{})
			source := sigTarball(t)
			if binary {
				source = binTarball(t, "")
			}
			extra := map[string]any{
				"pluginType": "cli", "binaryName": "nself-" + sigPlugin,
				"release_signature":   sigObj(signaturetest.Sign(t, signer, sigPlugin, "1.0.0", sha256Hex(source))),
				"platform_signatures": map[string]any{"linux-amd64": sigObj(signaturetest.SignPlatform(t, signer, sigPlugin, "1.0.0", "linux-amd64", sha256Hex(source)))},
			}
			sigServer(t, source, sha256Hex(source), extra)
			dir, err, _ := installSig(t)
			if codeOf(err) != "E121" {
				t.Fatalf("want E121, got %v", err)
			}
			if !dirEmpty(t, dir) {
				t.Fatal("source fallback was extracted")
			}
		})
	}
}
