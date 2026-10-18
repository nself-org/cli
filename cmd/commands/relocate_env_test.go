package commands

// Purpose: regression test for D-0905. The first `nself ci` of a v1.5 project
// auto-adds the ci plugin; loading the project config for that install exports
// the whole .env cascade (secrets included) into the process, and the child
// plugin used to inherit it. The child must see the shell environment only.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// envDumpScript writes the plugin's whole environment to $ENV_PROOF.
const envDumpScript = "#!/bin/sh\nenv > \"$ENV_PROOF\"\n"

func TestRelocateAutoAddKeepsProjectEnvOut(t *testing.T) {
	proof, _, _ := relocateSetup(t, "", nil)
	tb := aiTarball(t, map[string]string{"nself-ci": envDumpScript})
	sum := aiSum(tb)
	aiServe(t, tb, sum, aiSigned(t, sum))
	autoAddPlugin = autoInstallPlugin

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cascade := map[string]string{
		".env":         "D0905_DOTENV=from-dotenv\nD0905_SHELL=from-dotenv\n",
		".env.secrets": "D0905_SECRET=hunter2\n",
	}
	for name, body := range cascade {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("D0905_SHELL", "from-shell")
	// A key only the cascade sets must not outlive the call in this process either.
	t.Setenv("D0905_DOTENV", "")
	_ = os.Unsetenv("D0905_DOTENV")

	handled, err := relocateBreakout("ci", nil)
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	data, err := os.ReadFile(proof)
	if err != nil {
		t.Fatalf("the plugin did not run: %v", err)
	}
	child := string(data)
	for _, leak := range []string{"D0905_SECRET=", "D0905_DOTENV="} {
		if strings.Contains(child, leak) {
			t.Errorf("the plugin inherited the project cascade (%s)", leak)
		}
	}
	if !strings.Contains(child, "D0905_SHELL=from-shell") {
		t.Errorf("a .env value replaced the shell export for the plugin:\n%s", child)
	}
	if v, ok := os.LookupEnv("D0905_SECRET"); ok {
		t.Errorf("the cascade is still exported in the parent after auto-add: %q", v)
	}
	if got := os.Getenv("D0905_SHELL"); got != "from-shell" {
		t.Errorf("parent D0905_SHELL = %q, want the shell value", got)
	}
}
