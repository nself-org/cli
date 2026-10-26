package commands

// Purpose: keep restored machine-invocation secrets off mounted plugin argv.
// Inputs: synthetic secret bundles and a mounted binary that exits with 19.
// Outputs: E421 before execution for secret hand-offs, plugin exit otherwise.
// Constraints: shell fixture requires Unix; no real secrets or external services.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/mount"
	"github.com/spf13/cobra"
)

func mountedSecretProxy(t *testing.T) *cobra.Command {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	savedArgs, savedSecrets := os.Args, restoredInvokeSecrets
	t.Cleanup(func() { os.Args, restoredInvokeSecrets = savedArgs, savedSecrets })
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".nself", "plugins")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "nself-demo")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 19\n"), 0755); err != nil {
		t.Fatal(err)
	}
	node := &cobra.Command{Use: "demo"}
	proxyRunE(node, mount.Spec{Slug: "demo", BinaryPath: bin}, nil)
	return node
}

func TestMountProxyRestoredSecrets(t *testing.T) {
	node := mountedSecretProxy(t)
	const placeholder = "nself-invoke-secret:abcd1234abcd1234:0"
	for _, arg := range []string{placeholder, "--token=" + placeholder} {
		t.Run(arg, func(t *testing.T) {
			os.Args = []string{"nself", "demo", arg}
			t.Setenv(invoke.InvokedByEnv, "mcp")
			t.Setenv(invoke.SecretsEnv, `{"nonce":"abcd1234abcd1234","values":["FAKE-mounted-secret"]}`)
			if err := restoreSecretArgv(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(os.Args[2], "FAKE-mounted-secret") {
				t.Fatal("secret was not restored")
			}
			err := node.RunE(node, os.Args[2:])
			if err == nil || !strings.Contains(err.Error(), "E421") || strings.Contains(err.Error(), "FAKE-mounted-secret") {
				t.Fatalf("expected safe E421 before executing plugin, got %v", err)
			}
		})
	}
	// A following invocation must clear the previous hand-off state. Both
	// non-secret machine calls and direct calls must still execute the plugin.
	for _, invokedBy := range []string{"mcp", ""} {
		t.Setenv(invoke.InvokedByEnv, invokedBy)
		t.Setenv(invoke.SecretsEnv, "")
		os.Args = []string{"nself", "demo", "ordinary"}
		if err := restoreSecretArgv(); err != nil {
			t.Fatal(err)
		}
		var exit *plugin.ExitCodeError
		if err := node.RunE(node, os.Args[2:]); !errors.As(err, &exit) || exit.Code != 19 {
			t.Fatalf("non-secret call did not execute plugin: %v", err)
		}
	}
}
