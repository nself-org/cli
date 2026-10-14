package build

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
)

func entitlementFixture(t *testing.T, axis string) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fixture", "plugin.json"), []byte(`{"name":"fixture","license":"`+axis+`"}`), 0644); err != nil {
		t.Fatal(err)
	}
	return dir, []byte("services:\n  fixture:\n    image: fake/fixture\n    environment:\n      - EXISTING=yes\n      - NSELF_ENTITLEMENT_FILE=\n    volumes:\n      - data:/data\n      - fake:/run/nself/entitlement.jwt:rw\n  worker:\n    image: fake/worker\n")
}

func TestMountLicensedPlugin(t *testing.T) {
	compattest.Set(t, true)
	for _, axis := range []string{"licensed", "free"} {
		dir, in := entitlementFixture(t, axis)
		out, err := normalizeComposeEntitlement(in, dir, "fixture")
		if err != nil {
			t.Fatal(err)
		}
		if axis == "free" {
			if !bytes.Equal(out, in) {
				t.Fatal("free fragment changed")
			}
			continue
		}
		if strings.Count(string(out), entitlementMount) != 2 || strings.Count(string(out), entitlementContainerFile) != 4 {
			t.Fatalf("missing service guard: %s", out)
		}
		if strings.Contains(string(out), "fake:/run/nself") || strings.Contains(string(out), "NSELF_ENTITLEMENT_FILE=\n") {
			t.Fatal("fragment bypass survives")
		}
		again, err := normalizeComposeEntitlement(out, dir, "fixture")
		if err != nil || !bytes.Equal(again, out) {
			t.Fatal("mount must be idempotent")
		}
	}
}

func TestMountV14Unchanged(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir, in := entitlementFixture(t, "licensed")
		out, err := normalizeComposeEntitlement(in, dir, "fixture")
		if err != nil {
			t.Fatal(err)
		}
		if compat.V15() == bytes.Equal(in, out) {
			t.Fatal("compat mount decision incorrect")
		}
	})
}

func TestMountIgnoresOverride(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", strings.Repeat("01", 32))
	dir, in := entitlementFixture(t, "licensed")
	out, err := normalizeComposeEntitlement(in, dir, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if skipEntitlementMount() {
		if !bytes.Equal(out, in) {
			t.Fatal("tagged development mount changed")
		}
		return
	}
	if !bytes.Contains(out, []byte(entitlementMount)) {
		t.Fatal("release override skipped mount")
	}
}

func TestMountEmptyFileE125(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "")
	dir, in := entitlementFixture(t, "licensed")
	path := filepath.Join(dir, "fixture", pluginComposeFilename)
	if err := os.WriteFile(path, in, 0644); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	_, err := DiscoverPluginComposeFiles(project, dir)
	detail := errs.Describe(err)
	if detail == nil || detail.Code != "E125" || detail.ExitCode != 3 {
		t.Fatalf("want E125 exit3: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(project, ".nself", "state", "entitlement.jwt"))
	if err != nil || len(data) != 0 {
		t.Fatalf("empty file: %q %v", data, err)
	}
	fragment, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(fragment, []byte(entitlementMount)) {
		t.Fatalf("failed build omitted mount: %s %v", fragment, err)
	}
}

func TestMountRejectsDuplicateEnvironment(t *testing.T) {
	compattest.Set(t, true)
	dir, _ := entitlementFixture(t, "licensed")
	in := []byte("services:\n  fixture:\n    environment: {}\n    environment:\n      NSELF_ENTITLEMENT_FILE: ''\n")
	if _, err := normalizeComposeEntitlement(in, dir, "fixture"); err == nil {
		t.Fatal("duplicate env permits guard bypass")
	}
}

func TestMountPlanDoesNotPublish(t *testing.T) {
	compattest.Set(t, true)
	dir, in := entitlementFixture(t, "licensed")
	if err := os.WriteFile(filepath.Join(dir, "fixture", pluginComposeFilename), in, 0644); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if _, err := discoverPluginComposeFilesFx(&planEffects{}, newMemSink(project, ""), project, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project, ".nself", "state", "entitlement.jwt")); !os.IsNotExist(err) {
		t.Fatal("plan wrote token")
	}
}

// TestKeysEqual compares committed source data across the independent module boundary.
func TestKeysEqual(t *testing.T) {
	source, err := os.ReadFile("../../sdk/go/licensing/internal/keyset/keys.go")
	if os.IsNotExist(err) {
		source, err = os.ReadFile("../../sdk/go/licensing/keys.go")
	}
	if err != nil {
		t.Fatal(err)
	}
	rows := regexp.MustCompile(`ID:\s*"([^"]+)",\s*Public:\s*decode\("([0-9a-f]{64})"\)`).FindAllSubmatch(source, -1)
	if len(rows) != len(license.PingKeys) {
		t.Fatalf("SDK key count %d, CLI %d", len(rows), len(license.PingKeys))
	}
	for i, key := range license.PingKeys {
		if string(rows[i][1]) != key.ID || string(rows[i][2]) != hex.EncodeToString(key.Public) {
			t.Fatalf("SDK key ID or bytes differ at index %d", i)
		}
	}
}
