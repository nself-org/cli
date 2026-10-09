package build

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestComposeEnvManifest proves the build records the same ordered inputs that
// restart uses and that the computed plugin value resolves in Compose.
func TestComposeEnvManifest(t *testing.T) {
	f := newPlanFixture(t, "prod-plugins")
	writeFixtureFile(t, filepath.Join(f.plugins, "nself-alpha", "plugin.json"),
		`{"name":"nself-alpha","port":3901,"language":"go","dependencies":["nself-beta"]}`, 0o644)
	writeFixtureFile(t, filepath.Join(f.plugins, "nself-alpha", pluginComposeFilename),
		"services:\n  nself-alpha:\n    image: nself/alpha:latest\n    environment:\n      PLUGIN_VALUE: ${PLUGIN_NSELF_BETA_INTERNAL_URL}\n", 0o644)
	if _, err := Build(f.workdir, BuildOptions{}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := ComposeEnvFiles(f.workdir)
	if len(want) != 2 || want[0] != filepath.Join(f.workdir, ".env") ||
		want[1] != filepath.Join(f.workdir, composeEnvFile) {
		t.Fatalf("ComposeEnvFiles = %v", want)
	}
	got, err := ReadComposeEnvManifest(f.workdir)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadComposeEnvManifest = %v, %v; want %v", got, err, want)
	}
	manifest, err := os.ReadFile(filepath.Join(f.workdir, composeEnvManifestFile))
	if err != nil || string(manifest) != strings.Join(want, "\n")+"\n" {
		t.Fatalf("manifest = %q, %v", manifest, err)
	}
	info, err := os.Stat(filepath.Join(f.workdir, composeEnvManifestFile))
	if err != nil {
		t.Fatalf("stat manifest: %v", err)
	}
	// Windows has no Unix permission bits: a 0644 file reads back 0666.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Fatalf("manifest mode = %v", info.Mode().Perm())
	}
	computed, err := os.ReadFile(filepath.Join(f.workdir, composeEnvFile))
	if err != nil || !strings.Contains(string(computed), "PLUGIN_NSELF_BETA_INTERNAL_URL=http://plugin-nself-beta:3902") {
		t.Fatalf("computed plugin env missing: %v", err)
	}
	if runtime.GOOS != "linux" {
		return
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker compose unavailable")
	}
	args := []string{"compose"}
	for _, path := range got {
		args = append(args, "--env-file", path)
	}
	args = append(args, "-f", filepath.Join(f.plugins, "nself-alpha", pluginComposeFilename), "config", "--format", "json")
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose config: %v: %s", err, out)
	}
	var model struct {
		Services map[string]struct {
			Environment map[string]string `json:"environment"`
		} `json:"services"`
	}
	if err := json.Unmarshal(out, &model); err != nil {
		t.Fatalf("decode compose config: %v", err)
	}
	if got := model.Services["nself-alpha"].Environment["PLUGIN_VALUE"]; got != "http://plugin-nself-beta:3902" {
		t.Fatalf("PLUGIN_VALUE = %q", got)
	}
}

// TestComposeEnvManifestLegacy proves old projects get an empty path list.
func TestComposeEnvManifestLegacy(t *testing.T) {
	dir := t.TempDir()
	writeFixtureFile(t, filepath.Join(dir, ".env"), "LEGACY=1\n", 0o600)
	if files, err := ReadComposeEnvManifest(dir); err != nil || len(files) != 0 {
		t.Fatalf("missing legacy manifest = %v, %v", files, err)
	}
	if err := writeComposeEnvManifestVia(newDiskSink(dir), dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, composeEnvManifestFile))
	if err != nil || len(data) != 0 {
		t.Fatalf("legacy manifest = %q, %v", data, err)
	}
	if files, err := ReadComposeEnvManifest(dir); err != nil || len(files) != 0 {
		t.Fatalf("legacy manifest paths = %v, %v", files, err)
	}
}

// TestComposeEnvManifestStaleFailsClosed proves a missing or empty manifest is
// an error, not an empty list, once .nself/compose.env exists.
func TestComposeEnvManifestStaleFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		manifest *string
	}{
		{name: "missing"},
		{name: "empty", manifest: new(string)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFixtureFile(t, filepath.Join(dir, composeEnvFile), "PLUGIN_VAR=1\n", 0o600)
			if tc.manifest != nil {
				writeFixtureFile(t, filepath.Join(dir, composeEnvManifestFile), *tc.manifest, 0o644)
			}
			files, err := ReadComposeEnvManifest(dir)
			if err == nil || !strings.Contains(err.Error(), "run nself build") {
				t.Fatalf("ReadComposeEnvManifest = %v, %v; want stale-manifest error", files, err)
			}
		})
	}
	if files, err := ReadComposeEnvManifest(t.TempDir()); err != nil || files != nil {
		t.Fatalf("legacy project without compose.env = %v, %v; want nil, nil", files, err)
	}
}
