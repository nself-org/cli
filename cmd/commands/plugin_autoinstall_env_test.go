package commands

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestAutoInstallConfigPreservesBootstrapEnvironment(t *testing.T) {
	for _, state := range []string{"set", "empty", "unset"} {
		t.Run(state, func(t *testing.T) {
			t.Chdir(t.TempDir())
			keys := []string{"HOME", "NSELF_PLUGIN_DIR", "NSELF_PLUGIN_REGISTRY"}
			for _, key := range keys {
				value := ""
				if state == "set" {
					value = filepath.Join(t.TempDir(), key)
				}
				t.Setenv(key, value)
				if state == "unset" {
					_ = os.Unsetenv(key)
				}
			}
			if err := os.WriteFile(".env", []byte("PROJECT_NAME=fixture\nHOME=/project-home\nNSELF_PLUGIN_DIR=/project-plugins\nNSELF_PLUGIN_REGISTRY=https://project.invalid\n"), 0600); err != nil {
				t.Fatal(err)
			}
			_, _ = loadAutoInstallConfig()
			for _, key := range keys {
				got, present := os.LookupEnv(key)
				if present != (state != "unset") || (state != "set" && got != "") || (state == "set" && filepath.Base(got) != key) {
					t.Errorf("%s was overwritten: present=%v", key, present)
				}
			}
		})
	}
}

// TestAutoInstallPluginIgnoresHostileEnv drives autoInstallPlugin from a project
// whose .env tries to redirect the registry, HOME and plugin dir. The install must
// use the caller's locations: reverting autoInstallPlugin to loadConfig() makes the
// .env win, so the hostile registry is contacted and the plugin lands in its dir.
func TestAutoInstallPluginIgnoresHostileEnv(t *testing.T) {
	tb := aiTarball(t, nil)
	sum := aiSum(tb)
	aiServe(t, tb, sum, aiSigned(t, sum))
	goodDir := os.Getenv("NSELF_PLUGIN_DIR")

	var hostileHits int32
	hostile := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hostileHits, 1)
		http.NotFound(w, r)
	}))
	t.Cleanup(hostile.Close)
	hostileHome, hostilePlugins := t.TempDir(), t.TempDir()

	project := t.TempDir()
	env := "PROJECT_NAME=fixture\nHOME=" + hostileHome + "\nNSELF_PLUGIN_DIR=" + hostilePlugins + "\nNSELF_PLUGIN_REGISTRY=" + hostile.URL + "\n"
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)

	bin, err := autoInstallPlugin(aiSlug, "")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if n := atomic.LoadInt32(&hostileHits); n != 0 {
		t.Errorf("the project's .env redirected the registry: %d requests to it", n)
	}
	if entries, _ := os.ReadDir(hostilePlugins); len(entries) != 0 {
		t.Errorf("the project's .env redirected the plugin dir: %d entries written", len(entries))
	}
	if st, err := os.Stat(filepath.Join(goodDir, aiSlug, "plugin.json")); err != nil || st.IsDir() {
		t.Errorf("plugin not installed under the caller's plugin dir: %v", err)
	}
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		t.Errorf("published binary missing: %v", err)
	}
}
