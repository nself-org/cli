package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// collisionHome points HOME at a temp dir and returns its plugins directory.
func collisionHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return filepath.Join(home, ".nself", "plugins")
}

// extractFake writes plugin <name> into pluginsDir with its manifest and the
// given binaries, as an extracted tarball leaves it.
func extractFake(t *testing.T, pluginsDir, name, manifest string, bins ...string) string {
	t.Helper()
	dir := filepath.Join(pluginsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, manifest)
	for _, b := range bins {
		if err := os.WriteFile(filepath.Join(dir, b), []byte("#!/bin/sh\necho "+name+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func codeOfErr(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

// TestLinkCLIBinary_RefusesAnotherPluginsBinary is D-0259: a plugin whose
// cliCommands name a binary another installed plugin publishes is refused
// with E141 naming both plugins, and the other plugin's binary is untouched.
func TestLinkCLIBinary_RefusesAnotherPluginsBinary(t *testing.T) {
	pluginsDir := collisionHome(t)
	ownerDir := extractFake(t, pluginsDir, "alpha", `{"name":"alpha","version":"1.0.0","description":"fake","category":"utility","license":"MIT","cliCommands":[{"name":"media-server"}]}`, "nself-media-server")
	owner := &PluginManifest{Name: "alpha", CLICommands: []CLICommand{{Name: "media-server"}}}
	if err := linkCLIBinary(ownerDir, "alpha", owner); err != nil {
		t.Fatalf("first install: %v", err)
	}
	published := PublishedBinaryPath("nself-media-server")
	before, _ := os.ReadFile(published)

	dir := extractFake(t, pluginsDir, "beta", `{"name":"beta","version":"1.0.0","description":"fake","category":"utility","license":"MIT","cliCommands":[{"name":"beta"},{"name":"media-server"}]}`, "nself-beta", "nself-media-server")
	m := &PluginManifest{Name: "beta", CLICommands: []CLICommand{{Name: "beta"}, {Name: "media-server"}}}
	err := linkCLIBinary(dir, "beta", m)
	if codeOfErr(err) != "E141" {
		t.Fatalf("want E141, got %v", err)
	}
	for _, s := range []string{`"beta"`, `"alpha"`, "media-server"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error does not name %s: %v", s, err)
		}
	}
	after, _ := os.ReadFile(published)
	if string(after) != string(before) || !strings.Contains(string(after), "alpha") {
		t.Errorf("alpha's binary was replaced: %q", after)
	}
	if _, err := os.Stat(PublishedBinaryPath("nself-beta")); !os.IsNotExist(err) {
		t.Errorf("refused install published nself-beta anyway (stat err %v)", err)
	}
}

// TestLinkCLIBinary_ReinstallIsNotACollision: the plugin's own manifest is
// skipped, so an upgrade replaces its binary as before, and an orphan bin
// file no installed manifest claims is replaced too.
func TestLinkCLIBinary_ReinstallIsNotACollision(t *testing.T) {
	pluginsDir := collisionHome(t)
	dir := extractFake(t, pluginsDir, "alpha", `{"name":"alpha","version":"1.0.0","description":"fake","category":"utility","license":"MIT","cliCommands":[{"name":"media-server"}]}`, "nself-media-server")
	m := &PluginManifest{Name: "alpha", CLICommands: []CLICommand{{Name: "media-server"}}}
	for i := 0; i < 2; i++ {
		if err := linkCLIBinary(dir, "alpha", m); err != nil {
			t.Fatalf("install %d: %v", i, err)
		}
	}
	if err := os.WriteFile(PublishedBinaryPath("nself-orphan"), []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	odir := extractFake(t, pluginsDir, "orphan", `{"name":"orphan","version":"1.0.0","description":"fake","category":"utility","license":"MIT","pluginType":"cli"}`, "nself-orphan")
	if err := linkCLIBinary(odir, "orphan", &PluginManifest{Name: "orphan", PluginType: "cli"}); err != nil {
		t.Fatalf("orphan bin file blocked install: %v", err)
	}
}

// TestLinkCLIBinary_RefusesCoreVerbAndBadNames: the installer runs the
// manifestv2 name rules before writing anything (E113 core verb, E106 name;
// a name with a space is refused earlier by validateCLIBinaries).
func TestLinkCLIBinary_RefusesCoreVerbAndBadNames(t *testing.T) {
	pluginsDir := collisionHome(t)
	for _, c := range []struct {
		cmd, code string
	}{{"init", "E113"}, {"status", "E113"}, {"Feeds", "E106"}} {
		dir := extractFake(t, pluginsDir, "gamma", `{"name":"gamma","version":"1.0.0","description":"fake","category":"utility","license":"MIT"}`, "nself-init", "nself-status")
		err := linkCLIBinary(dir, "gamma", &PluginManifest{Name: "gamma", CLICommands: []CLICommand{{Name: c.cmd}}})
		if codeOfErr(err) != c.code {
			t.Errorf("%q: want %s, got %v", c.cmd, c.code, err)
		}
	}
	if _, err := os.Stat(PublishedBinaryPath("nself-init")); !os.IsNotExist(err) {
		t.Errorf("nself-init was published (stat err %v)", err)
	}
}
