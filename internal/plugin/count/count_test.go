package count

// Purpose: verify the embedded counts.json parses under the locked schema
// and that the advertised headline number is internally consistent with
// the free/pro/overlap breakdown it's derived from, per COUNTS-SCHEMA.md.
// Constraints: no network, no sibling registry checkouts — everything here
// exercises only the embedded artifact.

import (
	"encoding/json"
	"testing"
)

func TestLoad_ParsesEmbeddedArtifact(t *testing.T) {
	a, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if a.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", a.SchemaVersion)
	}
	if a.Sources.Free.Repo != "nself-org/plugins" {
		t.Errorf("Sources.Free.Repo = %q, want nself-org/plugins", a.Sources.Free.Repo)
	}
	if a.Sources.Pro.Repo != "nself-org/bundles" {
		t.Errorf("Sources.Pro.Repo = %q, want nself-org/bundles", a.Sources.Pro.Repo)
	}
}

// TestLoad_AdvertisedMatchesMeasuredGroundTruth pins the vendored snapshot
// to the ground truth recorded in COUNTS-SCHEMA.md on 2026-09-11: free
// 129/127, pro 46/46, overlap {cron,notify}, totals 173/171, advertised
// 171. If this ever fails after a legitimate resync, update the expected
// values here to match the new vendored counts.json — never the reverse.
func TestLoad_AdvertisedMatchesMeasuredGroundTruth(t *testing.T) {
	a, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if a.Free.Entries != 129 || a.Free.Installable != 127 {
		t.Errorf("Free = %+v, want entries=129 installable=127", a.Free)
	}
	if a.Pro.Entries != 46 || a.Pro.Installable != 46 {
		t.Errorf("Pro = %+v, want entries=46 installable=46", a.Pro)
	}
	if a.Totals.Entries != 173 || a.Totals.Installable != 171 {
		t.Errorf("Totals = %+v, want entries=173 installable=171", a.Totals)
	}
	if a.Advertised != 171 {
		t.Errorf("Advertised = %d, want 171", a.Advertised)
	}
}

// TestLoad_ArithmeticIsInternallyConsistent checks the derivation rule
// itself (not just the pinned numbers): totals.installable must equal
// free.installable + pro.installable minus the duplicate overlap slugs,
// and advertised must equal totals.installable per the locked schema.
func TestLoad_ArithmeticIsInternallyConsistent(t *testing.T) {
	a, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	wantInstallable := a.Free.Installable + a.Pro.Installable - len(a.Overlap.Duplicates)
	if a.Totals.Installable != wantInstallable {
		t.Errorf("Totals.Installable = %d, want %d (free.installable + pro.installable - len(duplicates))",
			a.Totals.Installable, wantInstallable)
	}
	if a.Advertised != a.Totals.Installable {
		t.Errorf("Advertised = %d, want it to equal Totals.Installable (%d)", a.Advertised, a.Totals.Installable)
	}
}

func TestRawJSON_IsValidAndMatchesLoad(t *testing.T) {
	raw := RawJSON()
	if len(raw) == 0 {
		t.Fatal("RawJSON() returned empty bytes")
	}

	var fromRaw Artifact
	if err := json.Unmarshal(raw, &fromRaw); err != nil {
		t.Fatalf("RawJSON() did not parse as JSON: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if fromRaw.Advertised != loaded.Advertised {
		t.Errorf("RawJSON advertised = %d, Load() advertised = %d, want equal", fromRaw.Advertised, loaded.Advertised)
	}
}

// TestRawJSON_ReturnsACopy guards against a caller's mutation of the
// returned slice corrupting the package-level embedded bytes for
// subsequent calls.
func TestRawJSON_ReturnsACopy(t *testing.T) {
	raw := RawJSON()
	if len(raw) == 0 {
		t.Fatal("RawJSON() returned empty bytes")
	}
	raw[0] = 'X'

	again := RawJSON()
	if again[0] == 'X' {
		t.Fatal("RawJSON() returned a shared slice — mutation leaked into the embedded artifact")
	}
}

func TestParse_RejectsUnsupportedSchemaVersion(t *testing.T) {
	bad := []byte(`{"schema_version": 2, "advertised": 171}`)
	if _, err := parse(bad); err == nil {
		t.Fatal("parse() with schema_version 2 should have errored, got nil")
	}
}

func TestParse_RejectsEmptyInput(t *testing.T) {
	if _, err := parse(nil); err == nil {
		t.Fatal("parse(nil) should have errored, got nil")
	}
}

func TestParse_RejectsMalformedJSON(t *testing.T) {
	if _, err := parse([]byte("not json")); err == nil {
		t.Fatal("parse() with malformed JSON should have errored, got nil")
	}
}
