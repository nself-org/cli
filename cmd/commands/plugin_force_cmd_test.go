package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/spf13/cobra"
)

// TestPluginInstallForceAcknowledgesSignatureBypass reaches the installer
// signature gate with the public flag alone, without the legacy FORCE env var.
func TestPluginInstallForceAcknowledgesSignatureBypass(t *testing.T) {
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
				if installErr != nil {
					t.Fatalf("--force did not reach signature verification: %v\n%s", installErr, stderr)
				}
				if !strings.Contains(stderr, "plugin signature verification skipped") {
					t.Fatalf("installer bypass acknowledgement missing:\n%s", stderr)
				}
				if _, err := os.Stat(aiPublished()); err != nil {
					t.Fatalf("plugin binary not installed: %v", err)
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
