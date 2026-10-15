package nself

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/portable"
)

func TestVerifiedReaderHandoff(t *testing.T) {
	ctx := context.Background()
	in := input(makeBundle(t))
	d, err := (Source{}).Detect(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if d.VerifiedBundle == nil {
		t.Fatal("Detect discarded its verified reader")
	}
	r := d.VerifiedBundle
	in.VerifiedBundle = r
	stage := filepath.Join(t.TempDir(), "stage")
	x, err := (Source{}).Extract(ctx, in, stage)
	if err != nil {
		t.Fatal(err)
	}
	if x.VerifiedBundle == nil {
		t.Fatal("Extract discarded the verified staged reader")
	}
	if got := x.VerifiedBundle; got == r || got.Dir() != stage {
		t.Fatalf("expected a separate staged reader, got %s", got.Dir())
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("detection JSON: %s, %v", b, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["VerifiedBundle"]; ok {
		t.Fatal("verified reader leaked into detection report")
	}
	// Replacing a member with identical bytes still violates the detected snapshot.
	p := filepath.Join(in.Values["dir"], "db", "data.dump")
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(p, filepath.Join(t.TempDir(), "old.dump")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Source{}).Extract(ctx, in, filepath.Join(t.TempDir(), "stage")); codeOf(err) != "E516" {
		t.Fatalf("Extract must retain Detect's file identity checks: %v", err)
	}
}

func TestReaderHandoffRejectsChangedManifest(t *testing.T) {
	ctx := context.Background()
	in := input(makeBundle(t))
	d, err := (Source{}).Detect(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	in.VerifiedBundle = d.VerifiedBundle
	m := d.VerifiedBundle.Manifest()
	m.Producer.Version = "changed"
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in.Values["dir"], portable.ManifestName), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Source{}).Extract(ctx, in, filepath.Join(t.TempDir(), "stage")); codeOf(err) != "E516" {
		t.Fatalf("changed manifest must fail staged verification: %v", err)
	}
	other := input(makeBundle(t))
	other.VerifiedBundle = d.VerifiedBundle
	if _, err := (Source{}).Extract(ctx, other, filepath.Join(t.TempDir(), "stage")); codeOf(err) != "E516" {
		t.Fatalf("reader for a different input must be refused: %v", err)
	}
}
