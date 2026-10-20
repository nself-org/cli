package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestTemplateLicenseKeysStoredSlots(t *testing.T) {
	for _, slot := range []int{2, 10} {
		t.Run(fmt.Sprint(slot), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
			t.Setenv("NSELF_OWNER_LICENSE_KEY", "")
			for i := 1; i <= 10; i++ {
				t.Setenv(fmt.Sprintf("NSELF_LICENSE_KEY_%d", i), "")
			}
			dir := filepath.Join(os.Getenv("HOME"), ".nself", "license")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			const fixture = "nself_plus_syntheticstored0000000000000"
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("key.%d", slot)), []byte(fixture), 0600); err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(templateLicenseKeys(), fixture) {
				t.Fatal("stored numbered slot missing")
			}
		})
	}
}
