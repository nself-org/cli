package controlplane

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

func FuzzInventoryLoad(f *testing.F) {
	f.Add([]byte("schema_version: 2\nproject: test\nenvironments:\n  local:\n    name: local\n    kind: local\n    servers: []\n"))
	f.Add([]byte("schema_version: 2\nenvironments:\n  prod:\n    name: prod\n    kind: remote\n    servers:\n      - name: app\n        host: '-oProxyCommand=touch /tmp/pwn'\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65536 {
			t.Skip()
		}
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, ".nself"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".nself", "control-plane.yaml"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		inv, err := Load(dir)
		if err != nil {
			return
		}
		for _, env := range inv.Environments {
			for _, s := range env.Servers {
				if s.Host != "" {
					if _, err := remote.ParseHostSpec(s.Host); err != nil {
						t.Fatalf("accepted invalid host %q: %v", s.Host, err)
					}
				}
			}
		}
	})
}
