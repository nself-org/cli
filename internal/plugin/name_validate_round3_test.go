package plugin

// Purpose: P7-PLUG-20 fix round 3 tests: reserved infrastructure names, the
// command proxy, manifest names and the 55-byte cap.
// Inputs: temp HOME and plugin dirs. Constraints: no Docker, no network.

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// mustBeReserved is the set derived from code (see reservedNames); listed here so
// emptying the map cannot make the tests below pass vacuously.
var mustBeReserved = []string{"bin", "common"}

func TestReservedNamesRefusedEverywhere(t *testing.T) {
	for _, name := range mustBeReserved {
		err := ValidateName(name)
		if d := errs.Describe(err); d == nil || d.Code != "E401" || !strings.Contains(err.Error(), "reserved") {
			t.Errorf("ValidateName(%q) = %v, want a reserved-name E401", name, err)
		}
		m := PluginManifest{Name: name, Version: "1.0.0"}
		if err := validateManifest(&m); !errors.Is(err, errs.ErrPluginManifest) {
			t.Errorf("validateManifest(%q) = %v, want ErrPluginManifest", name, err)
		}
		// Schema work is refused on its own, before any SQL (execPSQL would need Docker).
		if err := checkSchemaMapsBack(name, sanitizeSchemaName(name)); err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("checkSchemaMapsBack(%q) = %v, want a refusal", name, err)
		}
		if err := dropPluginSchema(context.Background(), &config.Config{ProjectName: "t"}, name); err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("dropPluginSchema(%q) = %v, want a refusal", name, err)
		}
	}
}

// TestRemoveRefusesReservedDirs: `plugin remove bin --keep-data --force` deleted
// every published command binary.
func TestRemoveRefusesReservedDirs(t *testing.T) {
	pluginDir := t.TempDir()
	for _, name := range mustBeReserved {
		dir := filepath.Join(pluginDir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "keep"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, keep := range []bool{true, false} {
			if err := Remove(context.Background(), &config.Config{}, name, pluginDir, keep, true); err == nil {
				t.Errorf("Remove(%q, keepData=%v) = nil", name, keep)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "keep")); err != nil {
			t.Errorf("Remove(%q) damaged the reserved dir: %v", name, err)
		}
	}
}

// TestInfrastructureEntriesNeverSlugs pins that the other non-plugin entries the
// CLI keeps in the plugin dir cannot be addressed as plugins.
func TestInfrastructureEntriesNeverSlugs(t *testing.T) {
	for _, n := range []string{".nself-state", ".install.lock", ".staging-thirdparty-123", ".bundle-rollback-123", "notify.prev"} {
		if err := ValidateName(n); err == nil {
			t.Errorf("ValidateName(%q) = nil", n)
		}
	}
}

// sharedSchemaRe finds schemas the CLI creates itself.
var sharedSchemaRe = regexp.MustCompile(`(?i)CREATE SCHEMA (?:IF NOT EXISTS )?"?(np_[a-z0-9_]+)`)

// TestReservedNamesCoverSharedSchemas fails when production code or SQL creates an
// np_ schema that a plugin name could map onto and the name is not reserved.
func TestReservedNamesCoverSharedSchemas(t *testing.T) {
	found := map[string]string{}
	for _, root := range []string{"..", filepath.Join("..", "..", "cmd")} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" || d.Name() == "vendor" {
					return fs.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, "_test.go") || !(strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".sql")) {
				return nil
			}
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			for _, m := range sharedSchemaRe.FindAllStringSubmatch(string(data), -1) {
				found[m[1]] = path
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := found["np_common"]; !ok {
		t.Fatalf("scan found no np_common schema (found %v): the scan is broken", found)
	}
	for schema, path := range found {
		name := strings.ReplaceAll(strings.TrimPrefix(schema, "np_"), "_", "-")
		if pluginSlugRe.MatchString(name) && ValidateName(name) == nil {
			t.Errorf("%s creates schema %s but plugin name %q is not reserved: add it to reservedNames", path, schema, name)
		}
	}
}

func TestValidateCommandName(t *testing.T) {
	for _, ok := range []string{"notify", "sentry-server", "bin", "common"} {
		if err := ValidateCommandName(ok); err != nil {
			t.Errorf("ValidateCommandName(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "..", "x/../../notify/run", "../notify", "a/b", `a\b`, "Notify", "-x", "a b"} {
		if d := errs.Describe(ValidateCommandName(bad)); d == nil || d.Code != "E401" {
			t.Errorf("ValidateCommandName(%q) accepted", bad)
		}
	}
}

// TestCommandProxyRefusesPathNames: "x/../../notify/run" resolved to
// <plugins>/notify/run, and "x/../notify" selected the notify manifest.
func TestCommandProxyRefusesPathNames(t *testing.T) {
	binDir, _ := binFixture(t)
	home := filepath.Dir(filepath.Dir(filepath.Dir(binDir)))
	// A file a path-shaped command would reach: <bin>/notify, and a plugin manifest
	// at <plugins>/notify declaring a variable the project .env supplies.
	if err := os.WriteFile(filepath.Join(binDir, "notify"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pdir := filepath.Join(filepath.Dir(binDir), "notify")
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"notify","version":"1.0.0","envVars":[{"name":"P20_PROXY_TEST_VAR"}]}`
	if err := os.WriteFile(filepath.Join(pdir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(home, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("P20_PROXY_TEST_VAR=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)

	if got := pluginEnvForCommand("notify"); len(got) == 0 {
		t.Skipf("positive control: the env fixture does not resolve for a valid name (%v)", got)
	}
	for _, name := range []string{"x/../notify", "x/../../notify/run", "../notify"} {
		if got := pluginEnvForCommand(name); got != nil {
			t.Errorf("pluginEnvForCommand(%q) read a manifest through a path: %v", name, got)
		}
		if IsCommandInstalled(name) {
			t.Errorf("IsCommandInstalled(%q) = true through a path", name)
		}
		if d := errs.Describe(ProxyCommand(name, nil)); d == nil || d.Code != "E401" {
			t.Errorf("ProxyCommand(%q) = %v, want E401", name, ProxyCommand(name, nil))
		}
	}
}

// TestManifestNameLengthCap: validateManifest and lifecycle use one rule, so a
// 56-byte name cannot be installed and then refused by remove.
func TestManifestNameLengthCap(t *testing.T) {
	ok := PluginManifest{Name: strings.Repeat("a", maxPluginNameLen), Version: "1.0.0"}
	if err := validateManifest(&ok); err != nil {
		t.Fatalf("a name of exactly the cap was refused: %v", err)
	}
	long := PluginManifest{Name: strings.Repeat("a", maxPluginNameLen+1), Version: "1.0.0"}
	if err := validateManifest(&long); !errors.Is(err, errs.ErrPluginManifest) {
		t.Fatalf("a %d-byte name: err = %v, want ErrPluginManifest", len(long.Name), err)
	}
}
