package plugin

// Purpose: P7-PLUG-20 fix round 1 tests: manifest binary names, lossy schema
// names, registry dependency names and the dotted-name narrowing.
// Inputs: temp HOME (the plugin bin dir lives under it), an httptest registry.
// Constraints: no Docker, no real network.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

func TestValidateBinaryName(t *testing.T) {
	for _, ok := range []string{"nself-foo", "nself-sentry-server", "tool.v2", "a_b"} {
		if err := ValidateBinaryName(ok); err != nil {
			t.Errorf("ValidateBinaryName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "../x", "../../../.ssh/authorized_keys", "a/b", `a\b`, "/etc/passwd", "-x", ".hidden", "a\x00b", "a\nb", "a b"} {
		err := ValidateBinaryName(bad)
		if d := errs.Describe(err); d == nil || d.Code != "E401" {
			t.Errorf("ValidateBinaryName(%q) = %v, want E401", bad, err)
		}
	}
}

// binFixture sets HOME and returns the bin dir plus a file OUTSIDE it that a
// traversal binaryName would reach: <home>/.nself/plugins/bin/../../victim.
func binFixture(t *testing.T) (binDir, victim string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	binDir = pluginBinDir()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	victim = filepath.Join(home, ".nself", "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	return binDir, victim
}

// TestUnlinkCLIBinaryRefusesTraversal: a manifest binaryName of "../../victim"
// used to make plugin remove delete a file outside the bin dir.
func TestUnlinkCLIBinaryRefusesTraversal(t *testing.T) {
	_, victim := binFixture(t)
	for _, m := range []*PluginManifest{
		{PluginType: "cli", BinaryName: "../../victim"},
		{PluginType: "cli", CLICommands: []CLICommand{{Name: "../../victim"}}},
	} {
		if err := unlinkCLIBinary("foo", m); err != nil {
			t.Fatalf("unlinkCLIBinary must skip, not fail the removal: %v", err)
		}
		if _, err := os.Stat(victim); err != nil {
			t.Fatalf("a file outside the bin dir was deleted: %v", err)
		}
	}
}

func TestLinkCLIBinaryRefusesTraversal(t *testing.T) {
	binDir, victim := binFixture(t)
	destDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(destDir, "x"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := &PluginManifest{PluginType: "cli", BinaryName: "../../victim"}
	err := linkCLIBinary(destDir, "foo", m)
	if d := errs.Describe(err); d == nil || d.Code != "E401" {
		t.Fatalf("linkCLIBinary = %v, want E401", err)
	}
	if b, rerr := os.ReadFile(victim); rerr != nil || string(b) != "keep" {
		t.Fatalf("victim file changed: %q %v", b, rerr)
	}
	if entries, _ := os.ReadDir(binDir); len(entries) != 0 {
		t.Fatalf("bin dir was written to: %v", entries)
	}
}

func TestSafePublishedBinaryPathContainment(t *testing.T) {
	binFixture(t)
	p, err := safePublishedBinaryPath("nself-foo")
	if err != nil || filepath.Dir(p) != pluginBinDir() {
		t.Fatalf("nself-foo -> %q, %v", p, err)
	}
	if _, err := safePublishedBinaryPath("../x"); err == nil {
		t.Fatal("../x accepted")
	}
}

func TestValidateManifestRejectsBadBinaryNames(t *testing.T) {
	base := func() PluginManifest {
		return PluginManifest{Name: "foo", Version: "1.0.0", PluginType: "cli"}
	}
	good := base()
	good.BinaryName = "nself-foo"
	if err := validateManifest(&good); err != nil {
		t.Fatalf("valid binaryName rejected: %v", err)
	}
	bad := base()
	bad.BinaryName = "../../../.ssh/authorized_keys"
	if err := validateManifest(&bad); !errors.Is(err, errs.ErrPluginManifest) {
		t.Fatalf("traversal binaryName: err = %v, want ErrPluginManifest", err)
	}
	bad2 := base()
	bad2.CLICommands = []CLICommand{{Name: "../x"}}
	if err := validateManifest(&bad2); !errors.Is(err, errs.ErrPluginManifest) {
		t.Fatalf("traversal cliCommands: err = %v, want ErrPluginManifest", err)
	}
}

// TestDropPluginSchemaRefusesLossyNames: "notify.prev" sanitizes to np_invalid,
// which is plugin "invalid"'s schema. No SQL may run (execPSQL would need Docker).
func TestDropPluginSchemaRefusesLossyNames(t *testing.T) {
	cfg := &config.Config{ProjectName: "t"}
	for _, name := range []string{"notify.prev", "a b", "x;y", "", "../x", "Notify", "my_plugin", "invalid"} {
		err := dropPluginSchema(context.Background(), cfg, name)
		if err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("dropPluginSchema(%q) = %v, want a refusal before any SQL", name, err)
		}
	}
}

// TestRemoveNeverTouchesDottedBackupDir: a *.prev backup is not a plugin.
func TestRemoveNeverTouchesDottedBackupDir(t *testing.T) {
	pluginDir := t.TempDir()
	prev := filepath.Join(pluginDir, "notify.prev")
	if err := os.MkdirAll(prev, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, keep := range []bool{true, false} {
		if err := Remove(context.Background(), &config.Config{}, "notify.prev", pluginDir, keep, true); err == nil {
			t.Fatalf("Remove(notify.prev, keepData=%v) = nil, want a refusal", keep)
		}
	}
	if _, err := os.Stat(prev); err != nil {
		t.Fatalf("backup dir removed: %v", err)
	}
}

// TestInstallLockedValidatesDependencyNamesFirst: a registry dependency of
// "../victim" that exists as a sibling dir used to be treated as "already
// installed"; it must be refused before any stat or download.
func TestInstallLockedValidatesDependencyNamesFirst(t *testing.T) {
	var downloads int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tarball") {
			atomic.AddInt32(&downloads, 1)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"plugins":[{"name":"baddep","version":"1.0.0","description":"fixture","category":"util","tier":"free","license":"MIT","dependencies":{"plugins":["../victim"]}}]}`))
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)

	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(filepath.Join(root, "victim"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := installLocked(context.Background(), &config.Config{ProjectName: "t"}, "baddep", pluginDir)
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E401" {
		t.Fatalf("installLocked = %v, want E401 for the dependency name", err)
	}
	if n := atomic.LoadInt32(&downloads); n != 0 {
		t.Fatalf("download hit %d times before the refusal", n)
	}
}

// TestNameLengthCapKeepsIdentifiersUnder63Bytes: two names that differ only past
// byte 63 of np_<name>_role would share one truncated schema and role.
func TestNameLengthCapKeepsIdentifiersUnder63Bytes(t *testing.T) {
	longest := strings.Repeat("a", maxPluginNameLen)
	if err := ValidateName(longest); err != nil {
		t.Fatalf("a name of exactly the cap was refused: %v", err)
	}
	if n := len(sanitizeSchemaName(longest) + "_role"); n > pgIdentMax {
		t.Fatalf("longest accepted name yields a %d-byte identifier", n)
	}
	cfg := &config.Config{ProjectName: "t"}
	for _, suffix := range []string{"x", "y"} {
		name := strings.Repeat("a", 60) + suffix
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName accepted a %d-byte name", len(name))
		}
		for label, err := range map[string]error{
			"drop":   dropPluginSchema(context.Background(), cfg, name),
			"create": createPluginSchema(context.Background(), cfg, name),
		} {
			if err == nil || !strings.Contains(err.Error(), "refusing") {
				t.Errorf("%s(%q) = %v, want a refusal before any SQL", label, name, err)
			}
		}
	}
	// checkSchemaMapsBack itself refuses an over-long generated identifier.
	if err := checkSchemaMapsBack("ab", "np_"+strings.Repeat("a", 70)); err == nil {
		t.Error("checkSchemaMapsBack accepted a mismatched schema")
	}
}

// TestInstalledTierIgnoresPathNames: `plugin info ../victim` read a manifest
// outside the plugin dir.
func TestInstalledTierIgnoresPathNames(t *testing.T) {
	pluginDir, victim := victimFixture(t)
	if err := os.WriteFile(filepath.Join(victim, "plugin.json"), []byte(`{"tier":"pro"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if tier, _, ok := InstalledTier(pluginDir, "../victim"); ok {
		t.Fatalf("InstalledTier read outside the plugin dir: tier %q", tier)
	}
}
