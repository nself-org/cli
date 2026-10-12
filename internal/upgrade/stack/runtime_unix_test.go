//go:build !windows

package stack

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestRuntimeReadinessIgnoresOtherProjectPlugins exercises the Unix Docker shim.
func TestRuntimeReadinessIgnoresOtherProjectPlugins(t *testing.T) {
	dir := t.TempDir()
	plugins := filepath.Join(dir, "plugins")
	other := filepath.Join(plugins, "other-project")
	if err := os.MkdirAll(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "plugin.json"), []byte("invalid outside this project"), 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncase \"$*\" in\n*config*) printf '%s' '{\"services\":{\"postgres\":{\"restart\":\"always\"}}}';;\n*ps*) printf '%s' '[]';;\n*) exit 1;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := NewRuntime(dir, &config.Config{PluginSystem: config.PluginSystemConfig{Dir: plugins}}, nil)
	if err := r.readiness(context.Background()); err != nil {
		t.Fatalf("unrelated plugin affected current project: %v", err)
	}
}
