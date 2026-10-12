package stack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestUpdateStackRuntimeSnapshotValidation(t *testing.T) {
	for _, tc := range []struct {
		name, data, want string
		mode             os.FileMode
		digest           bool
	}{
		{"pinned", `{"services":{"pg":{"image":"sha256:old","pull_policy":"never"}}}`, "", 0600, true},
		{"public", `{}`, "private regular file", 0644, true},
		{"changed digest", `{}`, "digest changed", 0600, false},
		{"invalid JSON", `{`, "decode recovery", 0600, true},
		{"empty services", `{"services":{}}`, "no services", 0600, true},
		{"mutable image", `{"services":{"pg":{"image":"pg:tag","pull_policy":"never"}}}`, "not pinned", 0600, true},
		{"build retained", `{"services":{"pg":{"image":"sha256:old","pull_policy":"never","build":"."}}}`, "not pinned", 0600, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "public" && runtime.GOOS == "windows" {
				t.Skip("Windows reports 0666 for every file; permission bits are not checked there")
			}
			r := NewRuntime(t.TempDir(), &config.Config{}, nil)
			state, err := r.privateState()
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(state, "update-stack-previous-proof.json")
			if err := os.WriteFile(path, []byte(tc.data), tc.mode); err != nil {
				t.Fatal(err)
			}
			j := &Journal{PreviousCompose: path}
			if tc.digest {
				sum := sha256.Sum256([]byte(tc.data))
				j.PreviousComposeSHA256 = hex.EncodeToString(sum[:])
			}
			err = r.verifySnapshot(j)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want=%s", err, tc.want)
			}
		})
	}
	r := NewRuntime(t.TempDir(), &config.Config{}, nil)
	if err := r.verifySnapshot(&Journal{PreviousCompose: "outside.json"}); err == nil {
		t.Fatal("accepted outside snapshot")
	}
	state, err := r.privateState()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.verifySnapshot(&Journal{PreviousCompose: filepath.Join(state, "update-stack-previous-missing.json")}); err == nil {
		t.Fatal("accepted missing snapshot")
	}
}

func TestUpdateStackRuntimeProductionRecoverySafety(t *testing.T) {
	for _, env := range []string{"prod", "production", "staging", "stage"} {
		t.Run(env, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "absent")
			cfg := &config.Config{Env: env}
			r := NewRuntime(dir, cfg, nil)
			if err := r.Restore(context.Background(), &Journal{}); err == nil {
				t.Fatal("automatic production restore accepted")
			}
			if err := r.RestartPrevious(context.Background(), &Journal{}); err == nil {
				t.Fatal("automatic production restart accepted")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("production refusal wrote to project")
			}
			if cfg.Backup.Dir != "" {
				t.Fatal("runtime mutated supplied config")
			}
		})
	}
}

func TestUpdateStackRuntimePluginDirectoryAndManifest(t *testing.T) {
	dir := t.TempDir()
	for _, pluginDir := range []string{"absent", filepath.Join(dir, "absent")} {
		r := NewRuntime(dir, &config.Config{PluginSystem: config.PluginSystemConfig{Dir: pluginDir}}, nil)
		if _, entries, err := r.pluginEntries(); err != nil || entries != nil {
			t.Fatalf("missing plugin dir: entries=%v err=%v", entries, err)
		}
	}
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir) // os.UserHomeDir reads this on Windows
	if err := os.Mkdir(filepath.Join(dir, "plugins"), 0700); err != nil {
		t.Fatal(err)
	}
	r := NewRuntime(dir, &config.Config{PluginSystem: config.PluginSystemConfig{Dir: "~/plugins"}}, nil)
	if got, _, err := r.pluginEntries(); err != nil || got != filepath.Join(dir, "plugins") {
		t.Fatalf("home path=%s err=%v", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not-directory"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	r.cfg.PluginSystem.Dir = "not-directory"
	if _, _, err := r.pluginEntries(); err == nil {
		t.Fatal("file used as plugin directory")
	}
	if m, err := loadReadinessManifest(dir, "missing"); err != nil || m != nil {
		t.Fatalf("missing manifest=%v err=%v", m, err)
	}
	plugin := filepath.Join(dir, "bad")
	if err := os.Mkdir(plugin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugin, "plugin.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadReadinessManifest(dir, "bad"); err == nil || !strings.Contains(err.Error(), "load installed plugin") {
		t.Fatalf("err=%v", err)
	}
}
