package entitlement

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestLicensedPluginAxes(t *testing.T) {
	for _, tc := range []struct {
		name, manifest     string
		licensed, rejected bool
	}{
		{"snake license", `{"requires_license":true}`, true, false},
		{"commercial", `{"isCommercial":true}`, true, false},
		{"max", `{"tier":"max"}`, true, false},
		{"plus", `{"tier":"plus"}`, true, false},
		{"enterprise", `{"tier":"enterprise"}`, true, false},
		{"business", `{"tier":"business"}`, true, false},
		{"free", `{"tier":"free"}`, false, false},
		{"empty", `{}`, false, false},
		{"v1 string licensed", `{"manifest_version":"1","tier":"max"}`, true, false},
		{"v1 decimal licensed", `{"manifest_version":1.0,"requires_license":true}`, true, false},
		{"v1 null licensed", `{"manifest_version":null,"isCommercial":true}`, true, false},
		{"v1 zero licensed", `{"manifest_version":0,"tier":"plus"}`, true, false},
		{"v1 string free", `{"manifest_version":"1","tier":"free"}`, false, false},
		{"v1 decimal free", `{"manifest_version":1.0}`, false, false},
		{"v1 null free", `{"manifest_version":null}`, false, false},
		{"v1 zero free", `{"manifest_version":0}`, false, false},
		{"v2 licensed", `{"manifest_version":2,"license":"licensed"}`, true, false},
		{"v2 string unsupported", `{"manifest_version":"2","license":"free"}`, false, true},
		{"v2 decimal unsupported", `{"manifest_version":2.0,"license":"free"}`, false, true},
		{"category object", `{"requires_license":true,"category":{"name":"security"}}`, true, false},
		{"category list", `{"requires_license":true,"category":["security"]}`, true, false},
		{"category number", `{"requires_license":true,"category":7}`, true, false},
		{"v2 free axis", `{"manifest_version":2,"license":"free","tier":"max","isCommercial":true}`, false, false},
		{"security v2", `{"manifest_version":2,"license":"licensed","category":"security"}`, false, true},
		{"security v1", `{"isCommercial":true,"category":"security"}`, false, true},
		{"malformed", `{`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(tc.manifest), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := IsLicensedPlugin(dir)
			if got != tc.licensed || (err != nil) != tc.rejected {
				t.Fatalf("licensed=%v err=%v", got, err)
			}
			if tc.rejected {
				if e := errs.Describe(err); e == nil || e.Code != "E125" {
					t.Fatalf("want E125: %v", err)
				}
			}
		})
	}
}

func TestLicensedInstalledIgnoresDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	pluginDir := filepath.Join(dir, "fixture")
	if err := os.Mkdir(pluginDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"plugin.json", ".disabled"} {
		if err := os.WriteFile(filepath.Join(pluginDir, file), []byte(`{"license":"licensed"}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := HasLicensedInstalled(); got || err != nil {
		t.Fatalf("disabled licensed=%v err=%v", got, err)
	}
}
