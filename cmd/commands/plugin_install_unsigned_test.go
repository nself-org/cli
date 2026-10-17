package commands

// Purpose: D-0842 — `nself plugin install` registers --allow-unsigned, and the
//          flag is refused when no URL install is requested (a registry
//          install can never opt out of signature checks).

import (
	"strings"
	"testing"
)

func TestPluginInstall_AllowUnsignedFlagRegistered(t *testing.T) {
	if pluginInstallCmd.Flags().Lookup("allow-unsigned") == nil {
		t.Fatal("plugin install has no --allow-unsigned flag")
	}
}

func TestPluginInstall_AllowUnsignedNeedsAURL(t *testing.T) {
	if err := pluginInstallCmd.Flags().Set("allow-unsigned", "true"); err != nil {
		t.Fatalf("setting --allow-unsigned: %v", err)
	}
	t.Cleanup(func() { _ = pluginInstallCmd.Flags().Set("allow-unsigned", "false") })

	err := runPluginInstall(pluginInstallCmd, []string{"ai"})
	if err == nil || !strings.Contains(err.Error(), "--allow-unsigned") {
		t.Fatalf("want a --allow-unsigned refusal for a registry install, got %v", err)
	}
}
