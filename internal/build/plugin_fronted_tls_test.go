package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx"
)

func TestFrontedPluginTLS(t *testing.T) {
	for _, served := range []bool{false, true} {
		t.Run(fmt.Sprint(served), func(t *testing.T) {
			root := t.TempDir()
			workdir := filepath.Join(root, "backend")
			cfg := &config.Config{BaseDomain: "example.com", SSLMode: "local"}
			cfg.Nginx.FrontedBy = filepath.Base(root)
			pluginDir := filepath.Join(root, "plugins")
			cfg.PluginSystem.Dir = pluginDir
			confDir := filepath.Join(pluginDir, "backup", "nginx")
			if err := os.MkdirAll(confDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(confDir, "backup.conf"), []byte(backupSnippet), 0644); err != nil {
				t.Fatal(err)
			}
			certRoot := workdir
			if served {
				certRoot = root
			}
			certDir := filepath.Join(certRoot, "ssl", "certificates", "example-com")
			if err := os.MkdirAll(certDir, 0755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"fullchain.pem", "privkey.pem"} {
				if err := os.WriteFile(filepath.Join(certDir, name), []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := 0
			if served {
				want = 1
			}
			hasSSL := nginx.NewGenerator(cfg, workdir).HasSSL()
			n, err := injectPluginNginxRoutesVia(newDiskSink(workdir), workdir, pluginDir, cfg, hasSSL)
			if err != nil || n != want {
				t.Fatalf("build injection = %d, %v; want %d", n, err, want)
			}
			n, err = InjectPluginNginxRoutes(workdir, pluginDir, cfg)
			if err != nil || n != want {
				t.Fatalf("public injection = %d, %v; want %d", n, err, want)
			}
			set := &snippetSet{}
			set.plugins(workdir, cfg, hasSSL)
			if len(set.files) != want {
				t.Errorf("model snippets = %d, want %d", len(set.files), want)
			}
			if served {
				out, err := os.ReadFile(filepath.Join(root, "nginx", "sites", "backup-backup.conf"))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(out), "/etc/nginx/ssl/certificates/example-com/fullchain.pem") {
					t.Error("wrong served certificate path")
				}
			}
		})
	}
}
