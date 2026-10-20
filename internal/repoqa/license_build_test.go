package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseBuildUsesCommittedLicenseKeys prevents build surfaces from
// advertising key injection that normal builds intentionally ignore (D-0227).
func TestReleaseBuildUsesCommittedLicenseKeys(t *testing.T) {
	for _, name := range []string{
		"Makefile", "Dockerfile", ".github/workflows/release.yml",
		".github/workflows/docker-publish.yml",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(repoRoot(t), name))
			if err != nil {
				t.Fatal(err)
			}
			for _, stale := range []string{"licensePubKeyHex", "NSELF_LICENSE_PUBKEY_HEX"} {
				if strings.Contains(string(data), stale) {
					t.Errorf("%s still advertises unsupported license key injection: %s", name, stale)
				}
			}
		})
	}
}
