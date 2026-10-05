package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTargetsMatchInstaller proves -targets prints every binary the installer
// publishes (P7-PLUG-55): all cliCommands of a multi-command plugin, v1 and v2
// alike, one binary for a single-command plugin, nothing for a non-cli plugin.
func TestTargetsMatchInstaller(t *testing.T) {
	cases := []struct{ name, rel, want string }{
		{"tenant v1", "v1/tenant.json", "nself-tenant\ttenant\nnself-billing\tbilling\n"},
		{"sentry-cli v1", "v1/sentry-cli.json", "nself-sentry\tsentry\nnself-sentry-server\tsentry-server\n"},
		{"tenant v2", "v2/tenant.json", "nself-tenant\ttenant\nnself-billing\tbilling\n"},
		{"sentry-cli v2", "v2/sentry-cli.json", "nself-sentry\tsentry\nnself-sentry-server\tsentry-server\n"},
		{"non-cli v1", "v1/ci.json", ""},
	}
	for _, c := range cases {
		code, out, e := runTool(t, "-in", copyFixture(t, c.rel), "-targets")
		if code != 0 || out != c.want {
			t.Errorf("%s: code %d out %q want %q (%s)", c.name, code, out, c.want, e)
		}
	}
}

// TestTargetsSingleCommandV2 covers a v2 plugin with one command and no
// compatibility keys: the commands block alone yields one target.
func TestTargetsSingleCommandV2(t *testing.T) {
	in := filepath.Join(t.TempDir(), "plugin.json")
	v1 := `{"name":"webhooks","version":"1.0.0","description":"d","category":"infrastructure","pluginType":"cli","binaryName":"nself-webhooks","cliCommands":[{"name":"webhooks","description":"d"}]}`
	if err := os.WriteFile(in, []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, e := runTool(t, "-in", in, "-targets"); code != 0 || out != "nself-webhooks\twebhooks\n" {
		t.Errorf("single v1: %d %q %s", code, out, e)
	}
	if code, conv, e := runTool(t, "-in", in); code != 0 {
		t.Fatalf("convert: %s", e)
	} else if err := os.WriteFile(in, []byte(conv), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, e := runTool(t, "-in", in, "-targets"); code != 0 || out != "nself-webhooks\twebhooks\n" {
		t.Errorf("single v2: %d %q %s", code, out, e)
	}
}
