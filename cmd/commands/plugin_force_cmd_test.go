package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/spf13/cobra"
)

// TestPluginInstallForceNeverSkipsRegistrySignature: --force with
// NSELF_LICENSE_SKIP_VERIFY reaches the installer gate, and the unsigned
// registry entry is still refused (E107; P7-PLUG-87 review H1).
func TestPluginInstallForceNeverSkipsRegistrySignature(t *testing.T) {
	for _, env := range []string{"dev", "prod", "staging"} {
		t.Run(env, func(t *testing.T) {
			compattest.Set(t, false)
			tarball := aiTarball(t, nil)
			aiServe(t, tarball, aiSum(tarball), nil)
			t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
			t.Setenv("NSELF_PLUGIN_INSTALL_TIER", "")
			t.Setenv("ENV", env)
			project := t.TempDir()
			if err := os.WriteFile(filepath.Join(project, ".env"), []byte("PROJECT_NAME=forcefixture\nENV="+env+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Chdir(project)
			cmd := &cobra.Command{Use: "install", RunE: runPluginInstall}
			cmd.Flags().Bool("force", false, "Acknowledge development bypass")
			cmd.SetArgs([]string{"--force", aiSlug})
			var installErr error
			stderr := captureStderr(t, func() { installErr = cmd.Execute() })
			if env == "dev" {
				if installErr == nil || !strings.Contains(stderr, "does not skip plugin signature checks") || !strings.Contains(stderr, "E107") {
					t.Fatalf("--force must reach the gate and the unsigned entry must be E107: %v\n%s", installErr, stderr)
				}
				if _, err := os.Stat(aiPublished()); err == nil {
					t.Fatal("an unsigned registry plugin was installed under --force")
				}
			} else if installErr == nil || !strings.Contains(stderr, "SECURITY:") {
				t.Fatalf("%s bypass must remain denied: %v\n%s", env, installErr, stderr)
			}
			if os.Getenv("NSELF_LICENSE_SKIP_VERIFY_FORCE") != "" {
				t.Fatal("command leaked its force acknowledgement into process environment")
			}
		})
	}
}
