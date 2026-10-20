package commands

// Purpose: D-1414. The auto-install seam must not let the project's .env redirect where it fetches from or installs to: NSELF_PLUGIN_REGISTRY, HOME and NSELF_PLUGIN_DIR keep the values the process had before the .env was loaded.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutoInstallIgnoresProjectEnvRedirects(t *testing.T) {
	tb := aiTarball(t, nil)
	sum := aiSum(tb)
	aiServe(t, tb, sum, aiSigned(t, sum))
	wantRegistry := os.Getenv("NSELF_PLUGIN_REGISTRY")
	wantHome := os.Getenv("HOME")
	wantDir := os.Getenv("NSELF_PLUGIN_DIR")

	project := t.TempDir()
	evilDir := filepath.Join(t.TempDir(), "evil-plugins")
	env := "NSELF_PLUGIN_REGISTRY=https://evil.example/registry\n" +
		"HOME=" + t.TempDir() + "\n" +
		"NSELF_PLUGIN_DIR=" + evilDir + "\n"
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)

	got, err := autoInstallPlugin(aiSlug, "")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if got != aiPublished() {
		t.Fatalf("published at %q, want %q", got, aiPublished())
	}
	if _, err := os.Stat(filepath.Join(wantDir, aiSlug, "plugin.json")); err != nil {
		t.Fatalf("not installed into the caller's NSELF_PLUGIN_DIR: %v", err)
	}
	if _, err := os.Stat(evilDir); err == nil {
		t.Fatalf("the project .env redirected the install to %s", evilDir)
	}
	for k, want := range map[string]string{"NSELF_PLUGIN_REGISTRY": wantRegistry, "HOME": wantHome, "NSELF_PLUGIN_DIR": wantDir} {
		if g := os.Getenv(k); g != want {
			t.Errorf("%s = %q after the call, want %q", k, g, want)
		}
	}
}
