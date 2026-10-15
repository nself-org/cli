package commands

// Purpose: prevent restored machine secrets reaching a mounted plugin's argv.
// Inputs: synthetic bundles and the mounted demo fixture. Outputs: E421 before
// exec for secret calls, successful exec for public machine and terminal calls.
// Constraints: no real secrets or external services.

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/plugin"
)

func TestMountRestoredSecretsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture is a shell script")
	}
	for _, flag := range []bool{false, true} {
		t.Run(map[bool]string{false: "positional", true: "flag"}[flag], func(t *testing.T) {
			dir := mountFixture(t)
			t.Setenv("NSELF_PLUGIN_DIR", dir)
			root, _ := mountRoot(t, dir)
			original := os.Args
			t.Cleanup(func() { os.Args = original; _ = restoreSecretArgv() })
			t.Setenv(invoke.InvokedByEnv, "mcp")
			t.Setenv(invoke.SecretsEnv, `{"nonce":"0123456789abcdef","values":["FAKE-D0651-secret"]}`)
			arg := "nself-invoke-secret:0123456789abcdef:0"
			if flag {
				arg = "--token=" + arg
			}
			os.Args = []string{"nself", "demo", "sub", arg}
			if err := restoreSecretArgv(); err != nil {
				t.Fatal(err)
			}
			root.SetArgs(os.Args[1:])
			_, err := root.ExecuteC()
			if err == nil || !strings.Contains(err.Error(), "E421") {
				t.Fatalf("want E421 before plugin exec, got %v", err)
			}
			if strings.Contains(err.Error(), "FAKE-D0651-secret") {
				t.Fatal("refusal disclosed the secret")
			}
			// A subsequent invocation without a bundle must clear the guard.
			os.Args = []string{"nself", "demo", "sub", "public"}
			if err := restoreSecretArgv(); err != nil {
				t.Fatal(err)
			}
			root.SetArgs(os.Args[1:])
			var exited *plugin.ExitCodeError
			if _, err := root.ExecuteC(); !errors.As(err, &exited) || exited.Code != 3 {
				t.Fatalf("public machine call must reach the fixture's exit 3: %v", err)
			}
			t.Setenv(invoke.InvokedByEnv, "")
			root.SetArgs([]string{"demo", "sub", "terminal-value"})
			if _, err := root.ExecuteC(); !errors.As(err, &exited) || exited.Code != 3 {
				t.Fatalf("terminal call must reach the fixture's exit 3: %v", err)
			}
		})
	}
}
