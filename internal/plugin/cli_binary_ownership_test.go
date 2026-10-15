package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// D-0603: publication and removal must preserve another installed plugin's
// binary, including when an on-disk manifest is edited after installation.
func TestCLIBinaryOwnership(t *testing.T) {
	for _, declaration := range []string{
		`,"pluginType":"cli","binaryName":"nself-notify"`,
		`,"cliCommands":[{"name":"other"},{"name":"notify"}]`,
	} {
		for _, action := range []string{"publish", "remove"} {
			t.Run(action+declaration, func(t *testing.T) {
				pluginsDir := collisionHome(t)
				ownerDir := extractFake(t, pluginsDir, "notify", `,"pluginType":"cli"`, "nself-notify")
				owner := readPluginManifest(ownerDir)
				if err := linkCLIBinary(ownerDir, "notify", owner); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(PublishedBinaryPath("nself-notify"))
				if err != nil {
					t.Fatal(err)
				}
				dir := extractFake(t, pluginsDir, "other", declaration, "nself-other", "nself-notify")
				m := readPluginManifest(dir)
				if m == nil {
					t.Fatal("invalid test manifest")
				}
				if action == "publish" {
					if err := linkCLIBinary(dir, "other", m); codeOfErr(err) != "E141" {
						t.Errorf("publication must refuse another owner's binary with E141: %v", err)
					}
					if _, err := os.Stat(PublishedBinaryPath("nself-other")); !os.IsNotExist(err) {
						t.Errorf("refused publication must not publish earlier targets: %v", err)
					}
				} else if err := unlinkCLIBinary("other", m); err != nil {
					t.Fatalf("removal should skip the foreign binary: %v", err)
				}
				after, err := os.ReadFile(PublishedBinaryPath("nself-notify"))
				if err != nil || string(after) != string(before) {
					t.Fatalf("%s changed notify's binary: %q, %v", action, after, err)
				}
			})
		}
	}
}

// Ownership checks must retain upgrades and multi-command plugins.
func TestCLIBinaryOwnershipAllowsOwnCommands(t *testing.T) {
	pluginsDir := collisionHome(t)
	dir := extractFake(t, pluginsDir, "sentry", `,"cliCommands":[{"name":"sentry"},{"name":"sentry-server"}]`, "nself-sentry", "nself-sentry-server")
	m := readPluginManifest(dir)
	for range 2 {
		if err := linkCLIBinary(dir, "sentry", m); err != nil {
			t.Fatal(err)
		}
	}
	for _, bin := range cliBinaryNames("sentry", m) {
		data, err := os.ReadFile(PublishedBinaryPath(bin))
		if err != nil || !strings.Contains(string(data), "echo sentry") {
			t.Fatalf("own command %s was not published: %q, %v", bin, data, err)
		}
	}
	if err := unlinkCLIBinary("sentry", m); err != nil {
		t.Fatal(err)
	}
	for _, bin := range cliBinaryNames("sentry", m) {
		if _, err := os.Stat(PublishedBinaryPath(bin)); !os.IsNotExist(err) {
			t.Errorf("own binary %s survived removal: %v", bin, err)
		}
	}
}

// A project plugin directory (NSELF_PLUGIN_DIR) is not ~/.nself/plugins: the
// owners come from the directory the plugin was extracted into, on both paths.
func TestCLIBinaryOwnershipProjectPluginDir(t *testing.T) {
	collisionHome(t)
	projectDir := filepath.Join(t.TempDir(), "plugins")
	ownerDir := extractFake(t, projectDir, "notify", `,"pluginType":"cli"`, "nself-notify")
	if err := linkCLIBinary(ownerDir, "notify", readPluginManifest(ownerDir)); err != nil {
		t.Fatal(err)
	}
	dir := extractFake(t, projectDir, "other", `,"pluginType":"cli","binaryName":"nself-notify"`, "nself-notify")
	m := readPluginManifest(dir)
	if err := linkCLIBinary(dir, "other", m); codeOfErr(err) != "E141" {
		t.Errorf("publication must refuse a binary owned in the project plugin dir: %v", err)
	}
	if err := unlinkCLIBinaryIn(projectDir, "other", m); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(PublishedBinaryPath("nself-notify")); err != nil {
		t.Errorf("removal unpublished notify's binary: %v", err)
	}
}
