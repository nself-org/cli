package plugin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

const v2FixtureDir = "manifestv2/testdata/v2"

func writeV2Manifest(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "plugin.json")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// differingFields lists the PluginManifest fields that differ, treating empty
// and nil slices, maps and pointers as equal.
func differingFields(a, b *PluginManifest) []string {
	var out []string
	va, vb := reflect.ValueOf(*a), reflect.ValueOf(*b)
	for i := 0; i < va.NumField(); i++ {
		x, y := va.Field(i), vb.Field(i)
		if isEmptyValue(x) && isEmptyValue(y) {
			continue
		}
		if !reflect.DeepEqual(x.Interface(), y.Interface()) {
			out = append(out, va.Type().Field(i).Name)
		}
	}
	return out
}

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Slice, reflect.Map:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	}
	return v.IsZero()
}

func TestLoadManifestV2Adapter(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(v2FixtureDir, "*.json"))
	if len(files) < 10 {
		t.Fatalf("expected the v2 fixtures, found %d", len(files))
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		got, err := LoadManifest(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		// What a released CLI decodes from the same bytes must equal what the
		// adapter builds from the v2 fields.
		var released PluginManifest
		if err := json.Unmarshal(data, &released); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if diff := differingFields(got, &released); len(diff) > 0 {
			t.Errorf("%s: adapter differs from the v1 decode in %v", f, diff)
		}
	}
	m, err := LoadManifest(filepath.Join(v2FixtureDir, "full.json"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "fixture-full" || m.License != "licensed" || !m.IsCommercial || m.Tier != "pro" || m.PublishStatus != "experimental" || m.Port != 3999 {
		t.Fatalf("canonical fields not mapped: %+v", m)
	}
	if m.Bundles != nil || m.TierPair || m.Checksum != "" {
		t.Fatalf("registry-only fields must stay empty: %+v", m)
	}
}

func TestLoadManifestV2TamperedCompat(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(v2FixtureDir, "tenant.json"))
	if err != nil {
		t.Fatal(err)
	}
	for key, val := range map[string]any{"status": "planned", "tier": "pro", "binaryName": "nself-evil", "isCommercial": true} {
		var doc map[string]any
		_ = json.Unmarshal(data, &doc)
		doc[key] = val
		tampered, _ := json.Marshal(doc)
		_, err := LoadManifest(writeV2Manifest(t, tampered))
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E112" {
			t.Errorf("tampered %s: want E112, got %v", key, err)
		}
	}
	// A v1 manifest still parses exactly as before.
	v1, err := os.ReadFile(filepath.Join("manifestv2/testdata/v1", "tenant.json"))
	if err != nil {
		t.Fatal(err)
	}
	if m, err := LoadManifest(writeV2Manifest(t, v1)); err != nil || m.License != "MIT" {
		t.Fatalf("v1 path changed: %v %+v", err, m)
	}
	// A version this release does not read is E114, not a silent v1 decode.
	var doc map[string]any
	_ = json.Unmarshal(data, &doc)
	doc["manifest_version"] = 3
	three, _ := json.Marshal(doc)
	_, err = LoadManifest(writeV2Manifest(t, three))
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E114" {
		t.Errorf("manifest_version 3: want E114, got %v", err)
	}
}
