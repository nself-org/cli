package commands

// Purpose: regression test for D-0980. With NSELF_V15=1 and NSELF_JSON_LEGACY=1
// the two account/licence paths whose pre-contract JSON was a different shape
// (license show with no key, account status from the cache) print that old
// shape, not the new v1 data document. Each shape has a golden.
// Inputs: package-level commands driven in v1.5 mode with --json, an isolated
// home, and an httptest account server that is down for the cached case.
// Constraints: NSELF_UPDATE_GOLDEN=1 rewrites the goldens; fixtures are fake.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/output"
)

// legacyGoldenDir is absolute because the fixtures change the working directory.
var legacyGoldenDir, _ = filepath.Abs(filepath.Join("testdata", "json", "legacy"))

// checkLegacyGolden compares out with testdata/json/legacy/<name>.golden.json.
func checkLegacyGolden(t *testing.T, name, out string) {
	t.Helper()
	path := filepath.Join(legacyGoldenDir, name+".golden.json")
	if os.Getenv("NSELF_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v", path, err)
	}
	if !bytes.Equal(want, []byte(out)) {
		t.Errorf("%s differs from %s:\n got %s\nwant %s", name, path, out, want)
	}
}

// TestLicenseShowNoKeyLegacyShape: the pre-contract no-key answer was
// {"error":"no license key configured"}, not the configured:false document.
func TestLicenseShowNoKeyLegacyShape(t *testing.T) {
	newLicenseFixture(t)
	t.Setenv(output.LegacyEnvVar, "1")
	out, err := runLicenseJSON(t, licenseShowCmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not bare JSON: %v\n%s", err, out)
	}
	if len(doc) != 1 || doc["error"] != "no license key configured" {
		t.Fatalf("license show --json without a key = %s, want only the old error key", out)
	}
	checkLegacyGolden(t, "license-show-no-key", out)
}

// TestAccountStatusCachedLegacyShape: the pre-contract cached answer carried
// cached=true and no status member.
func TestAccountStatusCachedLegacyShape(t *testing.T) {
	compattest.Set(t, true)
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	defer undo()
	home := isolateHome(t)
	t.Setenv(output.LegacyEnvVar, "1")
	writeTestAuthFile(t, home, &auth.AuthFile{AccessToken: fixtureAccessToken, Email: "alice@example.test", Tier: "plus", Bundles: []string{"chat"}})
	accountServer(t, false)
	out, err := runLicenseJSON(t, accountStatusCmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	noSecrets(t, "cached legacy status", out)
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not bare JSON: %v\n%s", err, out)
	}
	if doc["cached"] != true {
		t.Errorf("cached status lacks cached=true: %s", out)
	}
	if _, has := doc["status"]; has {
		t.Errorf("cached status carries the v1 status member: %s", out)
	}
	for _, k := range []string{"email", "display_name", "tier", "bundles"} {
		if _, has := doc[k]; !has {
			t.Errorf("cached status lacks %q: %s", k, out)
		}
	}
	checkLegacyGolden(t, "account-status-cached", out)
}
