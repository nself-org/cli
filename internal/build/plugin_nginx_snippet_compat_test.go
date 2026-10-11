package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

// D-0634: cli#673 expanded the default PLUGIN_DIR "~/.nself/plugins", so a
// v1.4 project upgraded in place got every installed plugin's server block
// on the next build, and a snippet the user had copied into nginx/sites by
// hand failed the build with a server_name conflict. v1.4 keeps the v1.4.13
// behaviour (PLUGIN_DIR as written, nothing found); v1.5 expands it.

// defaultPluginProject installs the backup snippet under an isolated
// ~/.nself/plugins, moves the working dir away from the package (v1.4
// resolves the literal "~/.nself/plugins" against it, as v1.4.13 did) and
// returns a project workdir plus the defaulted config.
func defaultPluginProject(t *testing.T) (string, *config.Config) {
	t.Helper()
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
	cfg, err := config.ApplyDefaults(&config.Config{ProjectName: "d0634", BaseDomain: "local.nself.org"})
	if err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}
	if cfg.PluginSystem.Dir != "~/.nself/plugins" {
		t.Fatalf("sanity: default PLUGIN_DIR = %q, want the literal ~/.nself/plugins", cfg.PluginSystem.Dir)
	}
	return t.TempDir(), cfg
}

// siteFiles lists nginx/sites under workdir (nil when it does not exist).
func siteFiles(t *testing.T, workdir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(workdir, "nginx", "sites"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestDefaultPluginDirDiscovery_V14Unchanged: on v1.4 a default-config build
// injects no plugin snippet, routes.json sees none, and ${NSELF_PLUGIN_DIR}
// stays "~/.nself/plugins", exactly as v1.4.13.
func TestDefaultPluginDirDiscovery_V14Unchanged(t *testing.T) {
	compattest.Set(t, false)
	workdir, cfg := defaultPluginProject(t)

	n, err := InjectPluginNginxRoutes(workdir, "", cfg)
	if err != nil || n != 0 {
		t.Fatalf("InjectPluginNginxRoutes = %d, %v; want 0, nil on v1.4", n, err)
	}
	for _, name := range siteFiles(t, workdir) {
		if strings.HasPrefix(name, "backup-") {
			t.Errorf("v1.4 build wrote plugin site file %s", name)
		}
	}

	set := &snippetSet{}
	set.plugins(workdir, cfg, true)
	if len(set.files) != 0 || len(set.global) != 0 {
		t.Errorf("v1.4 routes snippet set = %d files, global %v; want none", len(set.files), set.global)
	}

	if got := buildTemplateVars(workdir, cfg)["NSELF_PLUGIN_DIR"]; got != "~/.nself/plugins" {
		t.Errorf("v1.4 NSELF_PLUGIN_DIR = %q, want the literal ~/.nself/plugins", got)
	}
}

// TestDefaultPluginDirDiscovery_V15Injects: on v1.5 the same project gets
// the backup server block, routes.json sees it, and ${NSELF_PLUGIN_DIR} is
// the expanded home path.
func TestDefaultPluginDirDiscovery_V15Injects(t *testing.T) {
	compattest.Set(t, true)
	workdir, cfg := defaultPluginProject(t)

	n, err := InjectPluginNginxRoutes(workdir, "", cfg)
	if err != nil || n != 1 {
		t.Fatalf("InjectPluginNginxRoutes = %d, %v; want 1 snippet on v1.5", n, err)
	}
	if _, err := os.Stat(filepath.Join(workdir, "nginx", "sites", "backup-backup.conf")); err != nil {
		t.Errorf("v1.5 build did not write backup-backup.conf: %v", err)
	}

	set := &snippetSet{}
	set.plugins(workdir, cfg, true)
	if len(set.files) != 1 {
		t.Errorf("v1.5 routes snippet set = %d files, want 1", len(set.files))
	}

	home, _ := os.UserHomeDir()
	if got, want := buildTemplateVars(workdir, cfg)["NSELF_PLUGIN_DIR"], filepath.Join(home, ".nself", "plugins"); got != want {
		t.Errorf("v1.5 NSELF_PLUGIN_DIR = %q, want %q", got, want)
	}
}

// TestDefaultPluginDirDiscovery_HandCopiedSnippet: the readiness repro. A
// rendered plugin snippet the user copied into nginx/sites (what the snippet
// header tells them to do) keeps building on v1.4 and is left untouched; on
// v1.5 the injected copy claims the same server_name and the build stops.
func TestDefaultPluginDirDiscovery_HandCopiedSnippet(t *testing.T) {
	handCopy := strings.NewReplacer(
		"${BASE_DOMAIN}", "local.nself.org",
		"${SSL_DIR}", pluginSSLDir("local.nself.org"),
	).Replace(backupSnippet)

	compattest.Both(t, func(t *testing.T) {
		workdir, cfg := defaultPluginProject(t)
		sites := filepath.Join(workdir, "nginx", "sites")
		if err := os.MkdirAll(sites, 0o755); err != nil {
			t.Fatal(err)
		}
		copyPath := filepath.Join(sites, "backup.conf")
		if err := os.WriteFile(copyPath, []byte(handCopy), 0o644); err != nil {
			t.Fatal(err)
		}

		n, err := InjectPluginNginxRoutes(workdir, "", cfg)
		if compat.V15() {
			if err == nil || !strings.Contains(err.Error(), "server_name conflict") {
				t.Fatalf("v1.5 InjectPluginNginxRoutes = %d, %v; want a server_name conflict", n, err)
			}
			return
		}
		if err != nil || n != 0 {
			t.Fatalf("v1.4 InjectPluginNginxRoutes = %d, %v; want 0, nil", n, err)
		}
		got, err := os.ReadFile(copyPath)
		if err != nil || string(got) != handCopy {
			t.Errorf("v1.4 build changed the hand-copied snippet (err %v)", err)
		}
		if names := siteFiles(t, workdir); len(names) != 1 || names[0] != "backup.conf" {
			t.Errorf("v1.4 nginx/sites = %v, want only the hand copy", names)
		}
	})
}
