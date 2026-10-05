package manifestv2_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

// A v1 manifest_version of null, 0, "1", 1 or 1.0 is v1, not E114 (S4).
func TestManifestVersionV1Spellings(t *testing.T) {
	base := string(readFixture(t, "v1/ci.json"))
	for _, lit := range []string{`null`, `0`, `"1"`, `1`, `1.0`} {
		data := []byte(strings.Replace(base, "{", `{"manifest_version": `+lit+",", 1))
		m, err := manifestv2.ParseQuiet(data)
		if err != nil {
			t.Errorf("manifest_version %s: %v", lit, err)
			continue
		}
		if m.Name != "ci" || m.ManifestVersion != 2 {
			t.Errorf("manifest_version %s: not normalized to v2: %+v", lit, m.Name)
		}
		if !manifestv2.IsV1Version(json.RawMessage(lit)) {
			t.Errorf("IsV1Version(%s) = false", lit)
		}
	}
	for _, lit := range []string{`2`, `"2"`, `3`, `1.5`, `"0"`, `true`} {
		if manifestv2.IsV1Version(json.RawMessage(lit)) {
			t.Errorf("IsV1Version(%s) = true", lit)
		}
	}
}

// The normalizer accepts what v1.4.12's validateManifest accepts (S1): a file
// with no category or description, such as paid/internal/plugin.json. Validate
// stays strict for v2-native input.
func TestNormalizeAcceptsV1412Valid(t *testing.T) {
	data := []byte(`{"name":"internal","version":"1.0.0","language":"go","status":"stable","installable":false,
		"description":"Shared internal Go libraries."}`)
	m, err := manifestv2.Normalize(data)
	if err != nil {
		t.Fatalf("a file v1.4.12 accepts must normalize: %v", err)
	}
	if m.Category != "" || m.Name != "internal" || m.Installable == nil || *m.Installable {
		t.Errorf("unexpected normalized manifest: %+v", m)
	}
	wantCode(t, manifestv2.Validate(m), "E106", "category")
	// What v1.4.12 rejects still fails.
	for name, mut := range map[string]func(map[string]any){
		"name":     func(d map[string]any) { d["name"] = "Bad_Name" },
		"version":  func(d map[string]any) { d["version"] = "one" },
		"tables":   func(d map[string]any) { d["tables"] = []any{"users"} },
		"language": func(d map[string]any) { d["language"] = "cobol" },
		"status":   func(d map[string]any) { d["status"] = "bogus" },
	} {
		_, err := manifestv2.Normalize(mutate(t, "v1/ci.json", mut))
		wantCode(t, err, "E106", name)
	}
}

// Tier, licenseType and the licence text survive v1 -> v2 -> compat (S2, S3).
func TestLicensedValuesRoundTrip(t *testing.T) {
	for _, c := range []struct{ license, tier, lt string }{
		{"Source-Available", "max", "cloud"},
		{"Proprietary", "pro", "internal"},
		{"Source-Available", "max", "pro"},
	} {
		v1 := mutate(t, "v1/licensed.json", func(d map[string]any) { d["license"], d["tier"], d["licenseType"] = c.license, c.tier, c.lt })
		m, err := manifestv2.Normalize(v1)
		if err != nil {
			t.Fatal(err)
		}
		if m.License != "licensed" || m.LicenseSPDX != c.license || m.Tier != c.tier || m.LicenseType != c.lt || !m.IsCommercial || !m.RequiresLicense {
			t.Errorf("normalize %+v: got license=%s spdx=%s tier=%s licenseType=%s", c, m.License, m.LicenseSPDX, m.Tier, m.LicenseType)
		}
		out, err := manifestv2.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		back, err := manifestv2.Parse(out)
		if err != nil {
			t.Fatalf("%+v: v2 output does not load: %v", c, err)
		}
		if back.LicenseSPDX != c.license || back.Tier != c.tier || back.LicenseType != c.lt {
			t.Errorf("%+v: v2 round trip lost a value: %s %s %s", c, back.LicenseSPDX, back.Tier, back.LicenseType)
		}
		// -compat on a v2 file keeps the carried values.
		again, err := manifestv2.ApplyCompat(out)
		if err != nil || string(again) != string(out) {
			t.Errorf("%+v: ApplyCompat changed a canonical file: %v", c, err)
		}
	}
	// Licensed with no tier or licenseType stated: pro. Free stays free.
	m, err := manifestv2.Normalize(mutate(t, "v1/licensed.json", func(d map[string]any) { delete(d, "tier"); delete(d, "licenseType") }))
	if err != nil || m.Tier != "pro" || m.LicenseType != "pro" {
		t.Errorf("licensed default: %v %v %v", err, m.Tier, m.LicenseType)
	}
	if f, err := manifestv2.Normalize(readFixture(t, "v1/ci.json")); err != nil || f.Tier != "free" || f.LicenseType != "free" || f.LicenseSPDX != "MIT" {
		t.Errorf("free: %v %+v", err, f)
	}
	// A licensed manifest that tampers its tier to free still fails E112.
	bad := mutate(t, "v2/licensed.json", func(d map[string]any) { d["tier"] = "free" })
	_, err = manifestv2.Parse(bad)
	wantCode(t, err, "E112", "tier")
}

func TestUnmappedV1Keys(t *testing.T) {
	keys := []string{"config", "hooks", "actions", "routes", "migrations", "env", "notes", "api_version", "entry"}
	data := mutate(t, "v1/ci.json", func(d map[string]any) {
		for _, k := range []string{"actions", "config", "hooks"} {
			delete(d, k)
		}
		for _, k := range keys {
			d[k] = map[string]any{"a": 1}
		}
		d["empty_list"], d["empty_obj"], d["empty_str"], d["nothing"] = []any{}, map[string]any{}, "", nil
		d["bundles"], d["checksum"] = []any{"x"}, "abc"
	})
	got, err := manifestv2.UnmappedV1Keys(data)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, u := range got {
		names = append(names, u.Key)
		if u.Path != "$."+u.Key {
			t.Errorf("path of %s = %s", u.Key, u.Path)
		}
	}
	want := "actions,api_version,config,entry,env,hooks,migrations,notes,routes"
	if strings.Join(names, ",") != want {
		t.Errorf("unmapped = %v, want %s (empty values and registry keys are not listed)", names, want)
	}
	if reg := manifestv2.RegistryKeys(data); strings.Join(reg, ",") != "bundles,checksum" {
		t.Errorf("registry keys = %v", reg)
	}
	if s := got[0].String(); s != "$.actions (object, 1 entry)" {
		t.Errorf("String() = %q", s)
	}
	if clean, _ := manifestv2.UnmappedV1Keys(readFixture(t, "v1/status-stable.json")); len(clean) != 0 {
		t.Errorf("a clean fixture lists %v", clean)
	}
}
