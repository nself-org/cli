package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/nginx/plugincheck"
)

// markNginxPublic sets nginx.public in pluginDir/<name>/plugin.json, keeping
// any keys already there: the fixture's statement that the plugin
// authenticates every request its conf serves (D-0777).
func markNginxPublic(t *testing.T, pluginDir, name string) {
	t.Helper()
	path := filepath.Join(pluginDir, name, "plugin.json")
	m := map[string]any{"name": name}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	m["nginx"] = map[string]any{"public": true}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, path, string(data), 0o644)
}

// openBackupConf is the shape of plugins/free/backup/nginx/backup.conf: the
// whole plugin API on its own hostname, no access rule.
const openBackupConf = "server {\n    listen 80;\n    server_name backup.${BASE_DOMAIN};\n    set $upstream http://backup:3050;\n    location / { proxy_pass $upstream; }\n}\n"

const gatedCronConf = "server {\n    listen 80;\n    server_name cron.${BASE_DOMAIN};\n    allow 127.0.0.1;\n    allow ::1;\n    deny all;\n    set $upstream http://plugin-cron:3051;\n    location /v1/ { proxy_pass $upstream; }\n}\n"

func TestPluginNginxUngatedConfHeldBack(t *testing.T) {
	project, plugins := t.TempDir(), t.TempDir()
	writeFixtureFile(t, filepath.Join(plugins, "backup", "plugin.json"), `{"name":"backup","manifest_version":2}`, 0o644)
	writeFixtureFile(t, filepath.Join(plugins, "backup", "nginx", "backup.conf"), openBackupConf, 0o644)
	logs := captureSlog(t)

	n, err := InjectPluginNginxRoutes(project, plugins, minimalTestConfig("example.test"))
	if err != nil || n != 0 {
		t.Fatalf("inject = %d, %v; want 0 files", n, err)
	}
	if _, err := os.Stat(filepath.Join(project, "nginx", "sites", "backup-backup.conf")); !os.IsNotExist(err) {
		t.Fatalf("an ungated conf reached nginx/sites: %v", err)
	}
	for _, want := range []string{"left out of nginx/sites", "plugin=backup", "backup.conf", "access gate", `\"nginx\": {\"public\": true}`} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("warning lacks %q:\n%s", want, logs)
		}
	}
}

func TestPluginNginxPublicConfInjectedAsToday(t *testing.T) {
	project, plugins := t.TempDir(), t.TempDir()
	writeFixtureFile(t, filepath.Join(plugins, "stripe", "nginx", "stripe.conf"), openBackupConf, 0o644)
	markNginxPublic(t, plugins, "stripe")

	n, err := InjectPluginNginxRoutes(project, plugins, minimalTestConfig("example.test"))
	if err != nil || n != 1 {
		t.Fatalf("inject = %d, %v; want 1 file", n, err)
	}
	got, err := os.ReadFile(filepath.Join(project, "nginx", "sites", "stripe-stripe.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "server_name backup.example.test;") || strings.Contains(string(got), "deny all") {
		t.Errorf("public conf changed on the way in:\n%s", got)
	}
}

func TestPluginNginxGatedConfInjectedWithoutOptIn(t *testing.T) {
	project, plugins := t.TempDir(), t.TempDir()
	writeFixtureFile(t, filepath.Join(plugins, "cron", "nginx", "cron.conf"), gatedCronConf, 0o644)
	if n, err := InjectPluginNginxRoutes(project, plugins, minimalTestConfig("example.test")); err != nil || n != 1 {
		t.Fatalf("inject = %d, %v; want the loopback-gated conf injected", n, err)
	}
}

// An earlier build's copy goes when the conf is left out (a fronting stack's
// sites dir is not swept by this project's prune); another owner's file stays.
func TestPluginNginxHeldBackRemovesOwnCopyOnly(t *testing.T) {
	project, plugins := t.TempDir(), t.TempDir()
	cfg := minimalTestConfig("example.test")
	writeFixtureFile(t, filepath.Join(plugins, "backup", "nginx", "backup.conf"), openBackupConf, 0o644)
	writeFixtureFile(t, filepath.Join(plugins, "notify", "nginx", "notify.conf"), openBackupConf, 0o644)
	sites := filepath.Join(project, "nginx", "sites")
	own := stampNginxOwner(nginxGeneratedMarker+"\n"+openBackupConf, cfg.ProjectName+"/backup")
	foreign := stampNginxOwner(nginxGeneratedMarker+"\n"+openBackupConf, "other/notify")
	writeFixtureFile(t, filepath.Join(sites, "backup-backup.conf"), own, 0o644)
	writeFixtureFile(t, filepath.Join(sites, "notify-notify.conf"), foreign, 0o644)
	captureSlog(t)

	if _, err := InjectPluginNginxRoutes(project, plugins, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(sites, "backup-backup.conf")); !os.IsNotExist(err) {
		t.Errorf("this project's earlier copy of a left-out conf was kept: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(sites, "notify-notify.conf")); err != nil || string(got) != foreign {
		t.Errorf("another owner's file changed: %v", err)
	}
}

// routes.json records a conf left out for no gate the way it records a skipped one.
func TestPluginNginxHeldBackInRoutesJSON(t *testing.T) {
	workdir, plugins := t.TempDir(), t.TempDir()
	writeFixtureFile(t, filepath.Join(plugins, "backup", "nginx", "backup.conf"), openBackupConf, 0o644)
	st, base := snippetModel(t, workdir, plugins)
	m := st.withSnippetRoutes(base)
	for _, r := range m.Routes {
		if r.Source == "plugin" {
			t.Errorf("left-out conf modelled as a route: %+v", r)
		}
	}
	want := "skipped plugin backup/backup.conf: " + plugincheck.HoldReason
	for _, g := range m.UnmodelledGlobal {
		if g == want {
			return
		}
	}
	t.Errorf("unmodelled_global lacks %q: %q", want, m.UnmodelledGlobal)
}
