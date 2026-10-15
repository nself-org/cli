package nginx

import (
	"os"
	"path/filepath"
	"testing"
)

// D-0572: only the fronting stack's mounted pair can enable fronted TLS.
func TestFrontedTLS(t *testing.T) {
	for _, mode := range []string{"local", "custom", "letsencrypt", "none"} {
		for _, pair := range []string{"missing", "project", "served", "partial", "directory", "wrong-domain"} {
			t.Run(mode+"/"+pair, func(t *testing.T) {
				root := t.TempDir()
				workdir := filepath.Join(root, "backend")
				cfg := newCfgForSSLDetect(t, mode)
				cfg.Nginx.FrontedBy = filepath.Base(root)
				switch pair {
				case "project":
					writeCertPair(t, workdir, cfg.BaseDomain)
				case "served", "partial", "directory":
					writeCertPair(t, root, cfg.BaseDomain)
					key := filepath.Join(root, "ssl", "certificates", sslDirName(cfg.BaseDomain), "privkey.pem")
					if pair != "served" {
						if err := os.Remove(key); err != nil {
							t.Fatal(err)
						}
					}
					if pair == "directory" {
						if err := os.Mkdir(key, 0700); err != nil {
							t.Fatal(err)
						}
					}
				case "wrong-domain":
					writeCertPair(t, root, "owner.example.com")
				}
				want := pair == "served" && mode != "none"
				gen := NewGenerator(cfg, workdir)
				if got := gen.HasSSL(); got != want {
					t.Errorf("fronted HasSSL before assumptions = %v, want %v", got, want)
				}
				gen.WithAssumedCerts([]string{cfg.BaseDomain})
				if got := gen.HasSSL(); got != want {
					t.Errorf("fronted HasSSL = %v, want %v", got, want)
				}
				cfg.Nginx.FrontedBy = "unresolved-owner"
				if NewGenerator(cfg, workdir).HasSSL() {
					t.Error("unresolved topology enabled TLS")
				}
			})
		}
	}
}
