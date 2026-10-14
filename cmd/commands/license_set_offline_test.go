package commands

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
	"github.com/spf13/cobra"
)

func TestLicenseSetOutsideProjectOfflineSaved(t *testing.T) {
	for _, tc := range []struct {
		name               string
		transport, project bool
	}{{"503", false, false}, {"transport", true, false}, {"project-503", false, true}, {"project-transport", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
				for i := 1; i <= 10; i++ {
					t.Setenv("NSELF_LICENSE_KEY_"+strconv.Itoa(i), "")
				}
				t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))
				plugins := t.TempDir()
				t.Setenv("NSELF_PLUGIN_DIR", plugins)
				cwd := t.TempDir()
				if tc.project {
					if err := os.WriteFile(filepath.Join(cwd, ".env"), []byte("PROJECT_NAME=fixture\n"), 0600); err != nil {
						t.Fatal(err)
					}
					pluginDir := filepath.Join(plugins, "chat")
					if err := os.Mkdir(pluginDir, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{"manifest_version":2,"license":"licensed","name":"chat"}`), 0600); err != nil {
						t.Fatal(err)
					}
				}
				old, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Chdir(cwd); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chdir(old) })
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
				defer server.Close()
				if tc.transport {
					server.Close()
				}
				t.Setenv("NSELF_PING_API_URL", server.URL)
				cmd := &cobra.Command{}
				cmd.SetContext(context.Background())
				var warnings bytes.Buffer
				cmd.SetErr(&warnings)
				key := "nself_chat_abcdefghijklmnopqrstuvwxyz1234"
				out, err := captureStdout(t, func() error { return licenseSetCmd.RunE(cmd, []string{key}) })
				if err != nil {
					t.Fatalf("offline key save must succeed: %v", err)
				}
				if !strings.Contains(out, "license key saved.") {
					t.Fatalf("missing saved confirmation: %q", out)
				}
				keys := license.CollectLicenseKeys()
				if len(keys) != 1 || keys[0] != key {
					t.Fatalf("key not saved: %d keys", len(keys))
				}
				if strings.HasSuffix(t.Name(), "v1.5") && !strings.Contains(warnings.String(), "next build will retry") {
					t.Fatalf("missing retry warning: %q", warnings.String())
				}
			})
		})
	}
}
