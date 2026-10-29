package build

// plugin_configs_test.go — D-0478: ${SSL_DIR} renders to the base domain's
// certificate dir the CLI writes, and the default "~/.nself/plugins" plugin
// dir resolves under the home dir instead of the working dir (v1.5 only,
// D-0634).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
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
		if got := config.ExpandHomeDir(in); got != want {
			t.Errorf("ExpandHomeDir(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestInjectPluginNginxRoutes_DefaultDirRendersSSLDir runs the injection the
// way `nself build` does (pluginDir "", config defaults, so PLUGIN_DIR is the
// literal "~/.nself/plugins") and checks the backup snippet lands with the
// certificate path the CLI's own routes use, nothing left unrendered, and
// that the route model maps those lines to the base domain's ssl_dir. The
// default dir is expanded on v1.5 only (D-0634); v1.4 is covered in
// plugin_nginx_snippet_compat_test.go.
func TestInjectPluginNginxRoutes_DefaultDirRendersSSLDir(t *testing.T) {
	compattest.Set(t, true)
	home := t.TempDir()
	setHome(t, home)
	t.Chdir(t.TempDir())
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

// TestInjectPluginNginxRoutes_SkipsSnippetWithoutServedCert: when the build
// serves no TLS (SSL_MODE none, or letsencrypt before issuance), a snippet
// that loads the ${SSL_DIR} pair stays out of nginx/sites (nginx would not
// start on the missing file) and out of routes.json; one that does not
// load it is still injected.
func TestInjectPluginNginxRoutes_SkipsSnippetWithoutServedCert(t *testing.T) {
	pluginDir := t.TempDir()
	plain := "server {\n    listen 80;\n    server_name plain.${BASE_DOMAIN};\n    location / { return 204; }\n}\n"
	for name, body := range map[string]string{"backup": backupSnippet, "plain": plain} {
		d := filepath.Join(pluginDir, name, "nginx")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, name+".conf"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.ApplyDefaults(&config.Config{ProjectName: "d0478", BaseDomain: "example.com", SSLMode: "none"})
	if err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}
	cfg.PluginSystem.Dir = pluginDir
	for _, hasSSL := range []bool{false, true} {
		workdir := t.TempDir()
		n, err := injectPluginNginxRoutesVia(newDiskSink(workdir), workdir, pluginDir, cfg, hasSSL)
		if err != nil {
			t.Fatalf("hasSSL=%v: %v", hasSSL, err)
		}
		_, backupErr := os.Stat(filepath.Join(workdir, "nginx", "sites", "backup-backup.conf"))
		_, plainErr := os.Stat(filepath.Join(workdir, "nginx", "sites", "plain-plain.conf"))
		if wantN := map[bool]int{false: 1, true: 2}[hasSSL]; n != wantN || plainErr != nil || (backupErr == nil) != hasSSL {
			t.Errorf("hasSSL=%v: n=%d backup written=%v plain err=%v", hasSSL, n, backupErr == nil, plainErr)
		}
		set := &snippetSet{}
		set.plugins(workdir, cfg, hasSSL)
		if want := map[bool]int{false: 1, true: 2}[hasSSL]; len(set.files) != want {
			t.Errorf("hasSSL=%v: routes.json snippet files = %d, want %d", hasSSL, len(set.files), want)
		}
	}
}

func TestPluginSnippetSkip(t *testing.T) {
	vars := buildTemplateVars(t.TempDir(), &config.Config{BaseDomain: "example.com"})
	cert := "    ssl_certificate /etc/nginx/ssl/certificates/example-com/fullchain.pem;\n"
	cases := []struct {
		name, text string
		hasSSL     bool
		want       string // substring of the reason; "" = injected
	}{
		{"comment placeholder", "# ${NSELF_DOMAIN}\nserver { listen 80; }\n", false, ""},
		{"native uppercase variable", "map $host $UPSTREAM { default localhost; }\nserver { location / { proxy_pass http://${UPSTREAM}; } }\n", false, ""},
		{"native route variable", "map $host $CUSTOM_ROUTE { default localhost; }\nserver { location / { proxy_pass http://${CUSTOM_ROUTE}; } }\n", false, ""},
		{"valid server block", "server {\n    listen 80;\n    server_name a.example.com;\n    location / { return 204; }\n}\n", false, ""},
		{"nginx variable in braces", "server {\n    listen 80;\n    location / { return 301 https://${host}$request_uri; }\n}\n", false, ""},
		{"upstream and map at top level", "upstream u { server a:1; }\nmap $http_upgrade $c { default upgrade; }\nserver { listen 80; }\n", false, ""},
		{"unrendered placeholder", "server {\n    listen 443 ssl;\n    ssl_certificate /etc/nginx/ssl/${NSELF_DOMAIN}/fullchain.pem;\n}\n", true, "${NSELF_DOMAIN}"},
		{"unrendered route var", "server {\n    server_name ${GRAFANA_ROUTE}.example.com;\n}\n", true, "${GRAFANA_ROUTE}"},
		{"bare location block", "location /plugins/documents/ {\n    proxy_pass http://documents:3000;\n}\n", true, "top-level location"},
		{"does not parse", "server { listen 80;", true, "does not parse"},
		{"top-level set", "set $upstream http://a:1;\nserver { listen 80; }\n", true, "top-level set"},
		{"top-level cert pair is legal at http level", "ssl_certificate /etc/nginx/ssl/other/fullchain.pem;\nssl_certificate_key /etc/nginx/ssl/other/privkey.pem;\nserver { listen 80; }\n", true, ""},
		{"base cert, TLS served", "server {\n    listen 443 ssl;\n" + cert + "}\n", true, ""},
		{"base cert, no TLS served", "server {\n    listen 443 ssl;\n" + cert + "}\n", false, "TLS certificate"},
	}
	for _, c := range cases {
		got := pluginSnippetSkip(c.text, vars, c.hasSSL)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: pluginSnippetSkip = %q, want reason containing %q", c.name, got, c.want)
		}
	}
}
