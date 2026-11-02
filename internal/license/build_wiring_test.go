package license

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseBuildsDoNotInjectLicenseKey(t *testing.T) {
	for _, name := range []string{"Makefile", "Dockerfile", ".goreleaser.yml", ".github/workflows/release.yml", ".github/workflows/docker-publish.yml"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", name))
			if err != nil {
				t.Fatal(err)
			}
			for _, stale := range []string{"licensePubKeyHex", "NSELF_LICENSE_PUBKEY_HEX"} {
				if strings.Contains(string(data), stale) {
					t.Errorf("release build must use committed PingKeys, found %s", stale)
				}
			}
		})
	}
}
