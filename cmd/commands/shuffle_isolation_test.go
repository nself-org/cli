package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestInitFixturesIgnorePriorProject reproduces a project environment left by
// an earlier command test, without relying on a particular shuffle seed.
func TestInitFixturesIgnorePriorProject(t *testing.T) {
	t.Setenv("PROJECT_NAME", "invalid_project_name")
	t.Setenv("ENV", "prod")
	t.Setenv("BASE_DOMAIN", "previous.example")
	t.Run("domain", TestInitCmd_DomainFlagSetsBaseDomain)
	t.Run("template", TestInitCmd_TemplateFlag_Valid)
}

// TestInstalledWarningIgnoresPriorCompatMode reproduces a v1.5 environment
// inherited by the installed-binary legacy deprecation test.
func TestInstalledWarningIgnoresPriorCompatMode(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	t.Run("installed", TestInstalledBinaryEmitsDeprecationWarning)
}

// TestPluginEquivalenceIgnoresPriorFlags models a preceding dry-run invocation
// on the shared legacy command, followed by the equivalence fixture.
func TestPluginEquivalenceIgnoresPriorFlags(t *testing.T) {
	t.Cleanup(func() { resetFlags(pluginInstallCmd) })
	if err := pluginInstallCmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	t.Run("install", TestEquivalencePluginInstall)
}

// TestChecksumRegistryAfterReparenting models a throwaway execution root
// releasing commands that still appear in RootCmd's child list.
func TestChecksumRegistryAfterReparenting(t *testing.T) {
	t.Cleanup(reattachRealTree)
	root := &cobra.Command{Use: "fixture"}
	root.AddCommand(dbCmd)
	root.RemoveCommand(dbCmd)
	t.Run("checksum", TestResetChecksumConfirmBlock)
}

// TestBuildGoldenIgnoresPriorPassthrough reproduces dynamic config exported
// by an earlier fixture, which is absent from KnownEnvVars but affects compose.
func TestBuildGoldenIgnoresPriorPassthrough(t *testing.T) {
	t.Setenv("AUTH_PROVIDER_GOOGLE_ENABLED", "true")
	t.Run("golden", TestBuildPlanDevGolden)
}

// TestErrorHarnessRestoresCommandParents guards the producer of invalid parent
// pointers, so later registry tests do not each need a defensive repair.
func TestErrorHarnessRestoresCommandParents(t *testing.T) {
	reattachRealTree()
	t.Cleanup(reattachRealTree)
	t.Run("harness", func(t *testing.T) {
		if err := runErrorHarnessCmd(t, []string{"start", "--no-such-flag-xyz"}); err == nil {
			t.Fatal("the invalid harness invocation unexpectedly succeeded")
		}
	})
	for _, cmd := range RootCmd.Commands() {
		if cmd.Parent() != RootCmd {
			t.Errorf("%s retained a temporary execution root", cmd.Name())
		}
	}
}
