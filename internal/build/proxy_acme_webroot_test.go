package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProxyACMEWebroot(t *testing.T) {
	for _, provider := range []string{"nginx", "installed traefik", "declared traefik"} {
		t.Run(provider, func(t *testing.T) {
			f := freshProxyFixture(t)
			if provider == "installed traefik" {
				installProxy(t, f.plugins)
			} else if provider == "declared traefik" {
				declareProxy(t, f)
			}
			root := filepath.Join(f.workdir, "ssl", ".acme-webroot")
			if _, err := Build(f.workdir, BuildOptions{Mode: ModePlan}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatalf("plan created webroot: %v", err)
			}
			if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(root)
			if provider == "nginx" {
				if !os.IsNotExist(err) {
					t.Fatalf("nginx unexpectedly prepared webroot: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("build must create Traefik's webroot before Docker can: %v", err)
			}
			if !info.IsDir() || info.Mode().Perm()&0700 != 0700 {
				t.Fatalf("webroot is not an operator-writable directory: %v", info.Mode())
			}
			token := filepath.Join(root, "operator-token")
			if err := os.WriteFile(token, []byte("challenge"), 0600); err != nil {
				t.Fatalf("operator cannot write a challenge token: %v", err)
			}
			if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(token); err != nil || string(data) != "challenge" {
				t.Fatalf("rebuild changed existing webroot contents: %q, %v", data, err)
			}
		})
	}
}
