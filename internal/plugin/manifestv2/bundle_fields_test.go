package manifestv2_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

// fullWithBundle builds the bundle in the test: testdata/v2/full.json is read by
// other packages and stays free of a bundle block.
func fullWithBundle(t *testing.T, edit func(b *manifestv2.BundleProfile)) []byte {
	t.Helper()
	var m manifestv2.Manifest
	if err := json.Unmarshal(readFixture(t, "v2/full.json"), &m); err != nil {
		t.Fatal(err)
	}
	b := &manifestv2.BundleProfile{
		Provides:   []string{"chat-core"},
		Requires:   []manifestv2.Constraint{{Name: "auth", Range: ">=1.2.0 <2.0.0"}},
		Roles:      []manifestv2.Role{{ID: "server", Services: []string{"fixture-full", "auth"}}},
		Resources:  &manifestv2.Resources{MemoryMB: 256, DiskMB: 512, CPUMillis: 250},
		Migrations: []string{"migrations/001_init.sql"},
		Health:     "/health",
		Tier:       "chat",
	}
	if edit != nil {
		edit(b)
	}
	m.Bundle = b
	out, err := json.Marshal(&m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBundleMetadataContract(t *testing.T) {
	var m manifestv2.Manifest
	if err := json.Unmarshal(fullWithBundle(t, nil), &m); err != nil {
		t.Fatal(err)
	}
	out, err := manifestv2.Marshal(&m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := manifestv2.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bundle == nil || !reflect.DeepEqual(&m, got) {
		t.Fatalf("bundle does not round-trip: %#v", got.Bundle)
	}
	plain, err := manifestv2.Parse(readFixture(t, "v2/full.json"))
	if err != nil {
		t.Fatal(err)
	}
	if plain.Bundle != nil {
		t.Fatalf("manifest without a bundle block must parse to nil, got %#v", plain.Bundle)
	}
}

func TestBundleMetadataRejects(t *testing.T) {
	cases := []struct {
		name     string
		edit     func(b *manifestv2.BundleProfile)
		sentinel error
	}{
		{"empty range", func(b *manifestv2.BundleProfile) { b.Requires[0].Range = "" }, manifestv2.ErrBundleRange},
		{"bad range", func(b *manifestv2.BundleProfile) { b.Requires[0].Range = "latest" }, manifestv2.ErrBundleRange},
		{"negative memory", func(b *manifestv2.BundleProfile) { b.Resources.MemoryMB = -1 }, manifestv2.ErrBundleResource},
		{"negative disk", func(b *manifestv2.BundleProfile) { b.Resources.DiskMB = -1 }, manifestv2.ErrBundleResource},
		{"negative cpu", func(b *manifestv2.BundleProfile) { b.Resources.CPUMillis = -1 }, manifestv2.ErrBundleResource},
		{"empty tier", func(b *manifestv2.BundleProfile) { b.Tier = "" }, manifestv2.ErrBundleTier},
		{"bad tier", func(b *manifestv2.BundleProfile) { b.Tier = "Chat Bundle" }, manifestv2.ErrBundleTier},
		{"duplicate role", func(b *manifestv2.BundleProfile) { b.Roles = append(b.Roles, b.Roles[0]) }, manifestv2.ErrBundleRole},
		{"empty services", func(b *manifestv2.BundleProfile) { b.Roles[0].Services = nil }, manifestv2.ErrBundleRole},
		{"unknown service", func(b *manifestv2.BundleProfile) { b.Roles[0].Services = []string{"ghost"} }, manifestv2.ErrBundleRole},
		{"empty constraint name", func(b *manifestv2.BundleProfile) { b.Requires[0].Name = "" }, nil},
		{"bad provides slug", func(b *manifestv2.BundleProfile) { b.Provides = []string{"Chat Core"} }, nil},
		{"bad role id", func(b *manifestv2.BundleProfile) { b.Roles[0].ID = "Server" }, nil},
		{"bad migration", func(b *manifestv2.BundleProfile) { b.Migrations = []string{"../evil.sql"} }, nil},
		{"absolute migration", func(b *manifestv2.BundleProfile) { b.Migrations = []string{"/etc/passwd"} }, nil},
		{"bad health", func(b *manifestv2.BundleProfile) { b.Health = "health" }, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := manifestv2.Parse(fullWithBundle(t, c.edit))
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			if !errors.Is(err, errs.ErrPluginManifest) {
				t.Fatalf("want errs.ErrPluginManifest, got %v", err)
			}
			for _, s := range []error{manifestv2.ErrBundleRange, manifestv2.ErrBundleResource, manifestv2.ErrBundleTier, manifestv2.ErrBundleRole} {
				if want := s == c.sentinel; errors.Is(err, s) != want {
					t.Fatalf("errors.Is(%v) = %v, want %v: %v", s, !want, want, err)
				}
			}
		})
	}
}
