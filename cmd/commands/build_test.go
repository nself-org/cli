package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/sysexec"
	"github.com/spf13/cobra"
)

// newBuildCmd returns a fresh cobra.Command tree wired with just the build
// command so tests can execute it in isolation without side effects from the
// global RootCmd state.
func newBuildCmd() *cobra.Command {
	root := &cobra.Command{Use: "nself", RunE: func(cmd *cobra.Command, args []string) error { return nil }}

	bc := &cobra.Command{
		Use:   "build",
		Short: "Compose your infrastructure from .env",
		RunE:  runBuild,
	}
	bc.Flags().BoolP("force", "f", false, "Force rebuild all components")
	bc.Flags().Bool("no-cache", false, "Disable build cache")
	bc.Flags().BoolP("verbose", "v", false, "Show environment cascade")
	bc.Flags().Bool("debug", false, "Enable debug mode")
	bc.Flags().Bool("security-report", false, "Generate security analysis")
	bc.Flags().Bool("allow-insecure", false, "Allow insecure config (dev only)")
	bc.Flags().Bool("check", false, "Validate only, don't build")
	bc.Flags().BoolP("quiet", "q", false, "Suppress non-error output")
	bc.Flags().Bool("no-monorepo", false, "Disable automatic monorepo backend detection")
	bc.Flags().Bool("no-migration-check", false, "Skip v1 artifact detection")

	root.AddCommand(bc)
	return root
}

// TestBuildCmd_Registered verifies that the build subcommand is registered on
// the root command.
// isolateBuildCmd runs the test in an empty directory, so no project is found above
// the package dir, and keeps the build off the host's privileged tools (mkcert)
// with a fake runner: sysexec's guard panics on a machine that has mkcert installed.
func isolateBuildCmd(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Cleanup(sysexec.Set(&sysexec.Fake{}))
}

func TestBuildCmd_Registered(t *testing.T) {
	found := false
	for _, c := range RootCmd.Commands() {
		if c.Name() == "build" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'build' to be registered on RootCmd")
	}
}

// TestBuildCmd_FlagForce verifies that the --force flag is accepted by the
// build command without a parse error.
func TestBuildCmd_FlagForce(t *testing.T) {
	isolateBuildCmd(t)
	root := newBuildCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	// The command will fail because there is no nself project in the temp dir,
	// but the flag must be recognised (not an "unknown flag" error).
	root.SetArgs([]string{"build", "--force"})
	err := root.Execute()

	// We expect a project-not-found error, not a flag-parsing error.
	if err != nil && strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--force flag was not recognised: %v", err)
	}
}

// TestBuildCmd_FlagNoCache verifies that the --no-cache flag is accepted.
func TestBuildCmd_FlagNoCache(t *testing.T) {
	isolateBuildCmd(t)
	root := newBuildCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"build", "--no-cache"})
	err := root.Execute()

	if err != nil && strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--no-cache flag was not recognised: %v", err)
	}
}

// TestBuildCmd_FlagCheck verifies that the --check flag is accepted.
func TestBuildCmd_FlagCheck(t *testing.T) {
	isolateBuildCmd(t)
	root := newBuildCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"build", "--check"})
	err := root.Execute()

	if err != nil && strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--check flag was not recognised: %v", err)
	}
}

// TestBuildCmd_NoProject verifies that running build outside an nself project
// returns a meaningful error rather than panicking.
func TestBuildCmd_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newBuildCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	// Disable monorepo detection and migration check to reach FindNSelfRoot
	// as quickly as possible.
	root.SetArgs([]string{"build", "--no-monorepo", "--no-migration-check", "--quiet"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when running build outside an nself project, got nil")
	}
	if !strings.Contains(err.Error(), "nself init") && !strings.Contains(err.Error(), "nself project") {
		t.Errorf("expected error to mention 'nself init' or 'nself project', got: %v", err)
	}
}

// TestBuildCmd_UnknownFlagIsRejected verifies that an unrecognised flag is
// rejected with a cobra error rather than silently ignored.
func TestBuildCmd_UnknownFlagIsRejected(t *testing.T) {
	root := newBuildCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"build", "--this-flag-does-not-exist"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error for unknown flag, got nil")
	}
}
