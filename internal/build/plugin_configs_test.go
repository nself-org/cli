package build

// plugin_configs_test.go — D-0478: ${SSL_DIR} renders to the base domain's
// certificate dir the CLI writes, and the default "~/.nself/plugins" plugin
// dir resolves under the home dir instead of the working dir.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx/routemodel"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// backupSnippet is free/backup/nginx/backup.conf from nself-org/plugins
// (main, 2026-10-11), the snippet D-0478 was reproduced with.
const backupSnippet = `server {
    listen 443 ssl;
    http2 on;
    server_name backup.${BASE_DOMAIN};

    ssl_certificate /etc/nginx/ssl/${SSL_DIR}/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/${SSL_DIR}/privkey.pem;

    resolver 127.0.0.11 valid=10s;
    set $upstream http://backup:3050;

    location / {
        proxy_pass $upstream;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`

// setHome points os.UserHomeDir at dir on every platform.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestPluginSSLDir(t *testing.T) {
	cases := map[string]string{
		"local.nself.org": "certificates/local-nself-org",
		"example.com":     "certificates/example-com",
		"localhost":       "certificates/localhost",
		"":                "certificates/localhost",
	}
	for domain, want := range cases {
		if got := pluginSSLDir(domain); got != want {
			t.Errorf("pluginSSLDir(%q) = %q, want %q", domain, got, want)
		}
		vars := buildTemplateVars(t.TempDir(), &config.Config{BaseDomain: domain})
		if vars["SSL_DIR"] != want {
			t.Errorf("buildTemplateVars SSL_DIR for %q = %q, want %q", domain, vars["SSL_DIR"], want)
		}
	}
}

func TestExpandHomeDir(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	cases := map[string]string{
		"~/.nself/plugins": filepath.Join(home, ".nself", "plugins"),
		"~":                home,
		"/abs/plugins":     "/abs/plugins",
		"rel/plugins":      "rel/plugins",
		"~user/plugins":    "~user/plugins",
		"":                 "",
	}
	for in, want := range cases {
		if got := expandHomeDir(in); got != want {
			t.Errorf("expandHomeDir(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestInjectPluginNginxRoutes_DefaultDirRendersSSLDir runs the injection the
// way `nself build` does (pluginDir "", config defaults, so PLUGIN_DIR is the
// literal "~/.nself/plugins") and checks the backup snippet lands with the
// certificate path the CLI's own routes use, nothing left unrendered, and
// that the route model maps those lines to the base domain's ssl_dir.
func TestInjectPluginNginxRoutes_DefaultDirRendersSSLDir(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	confDir := filepath.Join(home, ".nself", "plugins", "backup", "nginx")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "backup.conf"), []byte(backupSnippet), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.ApplyDefaults(&config.Config{ProjectName: "d0478", BaseDomain: "local.nself.org"})
	if err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}
	if cfg.PluginSystem.Dir != "~/.nself/plugins" {
		t.Fatalf("sanity: default PLUGIN_DIR = %q, the case this test covers is the literal ~/.nself/plugins", cfg.PluginSystem.Dir)
	}
	workdir := t.TempDir()
	n, err := InjectPluginNginxRoutes(workdir, "", cfg)
	if err != nil || n != 1 {
		t.Fatalf("InjectPluginNginxRoutes = %d, %v; want 1 snippet from the default plugin dir", n, err)
	}
	out, err := os.ReadFile(filepath.Join(workdir, "nginx", "sites", "backup-backup.conf"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	want := "ssl_certificate " + nginxtopo.NginxSSLContainerPath + "/certificates/local-nself-org/fullchain.pem;"
	if !strings.Contains(got, want) {
		t.Errorf("rendered snippet lacks %q:\n%s", want, got)
	}
	if strings.Contains(got, "${") {
		t.Errorf("rendered snippet still holds a ${...} placeholder:\n%s", got)
	}
	routes, err := routemodel.FromSnippet("plugin", "backup", "backup-backup.conf", got)
	if err != nil || len(routes) != 1 {
		t.Fatalf("FromSnippet: %v routes=%d", err, len(routes))
	}
	r := routes[0]
	if r.TLS == nil || r.TLS.SSLDir != "local-nself-org" {
		t.Errorf("route TLS = %+v, want ssl_dir local-nself-org", r.TLS)
	}
	for _, u := range r.Unmodelled {
		if strings.Contains(u, "ssl_certificate") {
			t.Errorf("certificate line left unmodelled: %q", u)
		}
	}
}
