package clone

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestZoomCloneInstallableRequirements guards against a nonexistent plugin
// blocking template entitlement checks or appearing in the scaffolded config.
func TestZoomCloneInstallableRequirements(t *testing.T) {
	m, err := GetManifest("zoom-clone")
	if err != nil {
		t.Fatal(err)
	}
	s := &spy{grant: map[string]bool{"clawde": true}}
	rep := Check(context.Background(), m.Name, m.RequiredPlugins, fixtureCatalog(), s.lic)
	if err := rep.Err(); err != nil {
		t.Errorf("licensed zoom-clone cannot install: %v", err)
	}

	dir := t.TempDir()
	if _, err := Scaffold("zoom-clone", ScaffoldOptions{TargetDir: dir}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "nself.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Plugins []string `yaml:"plugins"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	want := []string{"auth", "livekit", "notify"}
	slices.Sort(m.RequiredPlugins)
	slices.Sort(config.Plugins)
	if !slices.Equal(m.RequiredPlugins, want) {
		t.Errorf("manifest plugins = %v, want %v", m.RequiredPlugins, want)
	}
	if !slices.Equal(config.Plugins, want) {
		t.Errorf("scaffold plugins = %v, want %v", config.Plugins, want)
	}
}
