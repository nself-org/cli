package commands

import (
	"github.com/nself-org/cli/internal/plugin"
	"os"
	"path/filepath"
	"testing"
)

func TestCICacheVersion(t *testing.T) {
	for _, version := range []string{"", "0.0.1", aiVersion} {
		t.Run("installed-"+version, func(t *testing.T) {
			cbEnv(t)
			cbRegistry(t, cbOpts{sign: true, platSign: true})
			pub := plugin.PublishedBinaryPath("nself-ci")
			if err := os.MkdirAll(filepath.Dir(pub), 0750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(pub, []byte("stale"), 0700); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(resolvePluginDir(), "ci")
			if err := os.MkdirAll(dir, 0750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"version":"`+version+`"}`), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := ensureCIBinary(false)
			if err != nil || got != pub {
				t.Fatalf("got %q, %v", got, err)
			}
			body, err := os.ReadFile(pub)
			if err != nil {
				t.Fatal(err)
			}
			if version != aiVersion && string(body) == "stale" {
				t.Fatal("stale published binary was trusted")
			}
		})
	}
}
