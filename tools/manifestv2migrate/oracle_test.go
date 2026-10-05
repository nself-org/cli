package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

const fixtureRoot = "../../internal/plugin/manifestv2/testdata"

// decodeV1 decodes bytes into the runtime PluginManifest, the v1.4.12 type
// (unchanged at the basis) that released CLIs fill from plugin.json.
func decodeV1(t *testing.T, path string) *plugin.PluginManifest {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m plugin.PluginManifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return &m
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

// differing lists PluginManifest fields that differ (empty equals nil).
func differing(a, b *plugin.PluginManifest) map[string]bool {
	out := map[string]bool{}
	va, vb := reflect.ValueOf(*a), reflect.ValueOf(*b)
	for i := 0; i < va.NumField(); i++ {
		x, y := va.Field(i), vb.Field(i)
		if isEmptyValue(x) && isEmptyValue(y) {
			continue
		}
		if !reflect.DeepEqual(x.Interface(), y.Interface()) {
			out[va.Type().Field(i).Name] = true
		}
	}
	return out
}

// TestFullStructOracle: every v1 fixture and its converted v2 file decode into
// plugin.PluginManifest with every field equal, except status and the
// licence-derived fields, which equal the D1 projection.
func TestFullStructOracle(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(fixtureRoot, "v1", "*.json"))
	if len(files) < 14 {
		t.Fatalf("expected the v1 fixtures, found %d", len(files))
	}
	// status and the licence-derived fields follow the D1 projection. The five
	// registry-only fields (Checksum, TierPair, Bundles, AuthorPublicKey,
	// Signature) are forbidden in a v2 file (E111), so the converter drops them
	// from a v1 file that carried them (licensed.json does).
	// tier and licenseType are carried (max stays max), so they are not exempt;
	// License is the v2 free|licensed key a released CLI reads as the licence text.
	exempt := map[string]bool{"PublishStatus": true, "License": true, "IsCommercial": true, "RequiresLicense": true,
		"Checksum": true, "TierPair": true, "Bundles": true, "AuthorPublicKey": true, "Signature": true}
	for _, f := range files {
		v1 := decodeV1(t, f)
		v2 := decodeV1(t, filepath.Join(fixtureRoot, "v2", filepath.Base(f)))
		for field := range differing(v1, v2) {
			if !exempt[field] {
				t.Errorf("%s: field %s differs between the v1 original and its v2 twin", filepath.Base(f), field)
			}
		}
		licensed := v1.IsCommercial || v1.RequiresLicense || (v1.LicenseType != "" && v1.LicenseType != "free") || (v1.Tier != "" && v1.Tier != "free")
		want := []any{"free", false, "free", false, "free"}
		if licensed {
			want = []any{"licensed", true, v1.LicenseType, true, v1.Tier}
		}
		got := []any{v2.License, v2.IsCommercial, v2.LicenseType, v2.RequiresLicense, v2.Tier}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: licence projection %v, want %v", filepath.Base(f), got, want)
		}
	}
}

// TestStatusTable: one fixture per row of the D1 status and maturity table.
func TestStatusTable(t *testing.T) {
	rows := []struct{ fixture, v1Status, maturity, state, compat string }{
		{"status-stable", "stable", "implemented", "", "stable"},
		{"status-beta", "beta", "implemented", "", "stable"},
		{"status-none", "", "implemented", "", "stable"},
		{"status-experimental", "experimental", "experimental", "", "experimental"},
		{"status-alpha", "alpha", "experimental", "", "experimental"},
		{"status-planned", "planned", "planned", "", "planned"},
		{"status-deprecated", "deprecated", "deferred", "deprecated", "deprecated"},
		{"status-eol", "eol", "deferred", "eol", "eol"},
	}
	for _, r := range rows {
		v1Path := filepath.Join(fixtureRoot, "v1", r.fixture+".json")
		m, err := manifestv2.ParseQuiet(mustRead(t, v1Path))
		if err != nil {
			t.Fatalf("%s: %v", r.fixture, err)
		}
		state := ""
		if m.Deprecation != nil {
			state = m.Deprecation.State
		}
		if m.Maturity != r.maturity || state != r.state || m.Status != r.compat {
			t.Errorf("%s (%s): got maturity=%s state=%s compat=%s, want %s %s %s", r.fixture, r.v1Status, m.Maturity, state, m.Status, r.maturity, r.state, r.compat)
		}
		// v1 -> v2 file -> v1 compat: the released type reads the table's status.
		if got := decodeV1(t, filepath.Join(fixtureRoot, "v2", r.fixture+".json")).PublishStatus; got != r.compat {
			t.Errorf("%s: v2 file decodes to status %q, want %q", r.fixture, got, r.compat)
		}
	}
	// The one row no v1 value reaches: scaffolded projects to experimental.
	var doc map[string]any
	_ = json.Unmarshal(mustRead(t, filepath.Join(fixtureRoot, "v2", "full.json")), &doc)
	doc["maturity"] = "scaffolded"
	b, _ := json.Marshal(doc)
	fixed, err := manifestv2.ApplyCompat(b)
	if err != nil {
		t.Fatal(err)
	}
	if m, err := manifestv2.Parse(fixed); err != nil || m.Status != "experimental" {
		t.Errorf("scaffolded: %v %+v", err, m)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
