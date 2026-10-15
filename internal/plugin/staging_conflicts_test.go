package plugin

// Purpose: Keep URL-install staging manifests out of installed-plugin conflicts.
// Inputs: Local HTTP archives and temporary plugin directories.
// Outputs: Successful installs and retained conflicts with real installations.
// Constraints: Docker is replaced by a local SQL-command stub; no running stack.
// SPORT: third-party install and installed-plugin conflict scanners.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestInstallFromURL_StagingDoesNotConflict(t *testing.T) {
	for _, tables := range []bool{true, false} {
		name := "routes-only"
		if tables {
			name = "tables-and-routes"
		}
		t.Run(name, func(t *testing.T) {
			stubStagingInstallDocker(t)
			manifest := map[string]any{
				"name": "third-party-demo", "version": "1.0.0",
				"description": "Staging regression", "category": "utility", "license": "MIT",
				"apiEndpoints": []string{"demo/health"},
			}
			if tables {
				manifest["tables"] = []string{"np_demo_events"}
			}
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			archive := buildThirdPartyTarball(t, string(data))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(archive)
			}))
			defer server.Close()
			dir := t.TempDir()
			if err := InstallFromURL(context.Background(), &config.Config{}, server.URL+"/plugin.tar.gz", dir, ""); err != nil {
				t.Fatalf("install into empty directory: %v", err)
			}
			installed, err := parseManifest(filepath.Join(dir, "third-party-demo", "plugin.json"))
			if err != nil {
				t.Fatalf("installed manifest: %v", err)
			}
			if len(installed.APIEndpoints) != 1 {
				t.Fatalf("installed routes = %v, want demo/health", installed.APIEndpoints)
			}
		})
	}
}

func stubStagingInstallDocker(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	// Only the schema-version query needs a value. Other SQL commands succeed.
	script := "#!/bin/sh\ncase \"$*\" in\n  *'SELECT COALESCE(MAX(version),0)'*) printf '0\\n' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	setTestHome(t, t.TempDir())
}

func TestTableConflicts_IgnoreStagingKeepInstalled(t *testing.T) {
	for _, dirName := range []string{".staging-thirdparty-123", "other-plugin"} {
		t.Run(dirName, func(t *testing.T) {
			dir := t.TempDir()
			writeManifestFileAs(t, dir, dirName, "other-plugin", "1.0.0", []string{"np_demo_events"}, false)
			manifest := &PluginManifest{Name: "third-party-demo", Tables: []string{"np_demo_events"}}
			wantConflict := dirName == "other-plugin"
			for name, err := range map[string]error{
				"prefix": checkTablePrefixConflict(dir, manifest.Name, manifest.Tables),
				"exact":  checkTableConflicts(dir, manifest),
			} {
				if (err != nil) != wantConflict {
					t.Errorf("%s conflict = %v, want conflict %v", name, err, wantConflict)
				}
			}
		})
	}
}
