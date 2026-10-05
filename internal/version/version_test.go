package version

import (
	"testing"
)

// TestGetVersion_NonEmpty verifies that GetVersion returns a non-empty string.
func TestGetVersion_NonEmpty(t *testing.T) {
	v := GetVersion()
	if v == "" {
		t.Error("GetVersion returned empty string — Version var must be set")
	}
}

// TestGetCommit_NonEmpty verifies that GetCommit returns a non-empty string.
func TestGetCommit_NonEmpty(t *testing.T) {
	c := GetCommit()
	if c == "" {
		t.Error("GetCommit returned empty string — Commit var must be set")
	}
}

// TestGetBuildDate_NonEmpty verifies that GetBuildDate returns a non-empty string.
func TestGetBuildDate_NonEmpty(t *testing.T) {
	d := GetBuildDate()
	if d == "" {
		t.Error("GetBuildDate returned empty string — BuildDate var must be set")
	}
}

// TestVersion_Overridable verifies that the package-level vars can be overridden
// (simulates what ldflags does at build time).
func TestVersion_Overridable(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()

	Version = "9.9.9"
	if GetVersion() != "9.9.9" {
		t.Errorf("expected GetVersion() = 9.9.9 after override, got %q", GetVersion())
	}
}

// TestCommit_Overridable verifies Commit override.
func TestCommit_Overridable(t *testing.T) {
	orig := Commit
	defer func() { Commit = orig }()

	Commit = "deadbeef"
	if GetCommit() != "deadbeef" {
		t.Errorf("expected GetCommit() = deadbeef after override, got %q", GetCommit())
	}
}

// TestBuildDate_Overridable verifies BuildDate override.
func TestBuildDate_Overridable(t *testing.T) {
	orig := BuildDate
	defer func() { BuildDate = orig }()

	BuildDate = "2026-01-01"
	if GetBuildDate() != "2026-01-01" {
		t.Errorf("expected GetBuildDate() = 2026-01-01 after override, got %q", GetBuildDate())
	}
}

// TestNextMinor verifies the escape-hatch removal-version arithmetic used by
// NSELF_LEGACY_ENV_ORDER (CLI-R18): patch resets to 0, minor increments.
func TestNextMinor(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"1.2.7", "1.3.0"},
		{"1.2.0", "1.3.0"},
		{"0.9.9", "0.10.0"},
		{"2.0.0", "2.1.0"},
	}
	for _, c := range cases {
		if got := NextMinor(c.in); got != c.want {
			t.Errorf("NextMinor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNextMinor_UnparseableReturnsUnchanged verifies that a version string
// that isn't three dot-separated integers is returned as-is rather than
// panicking or producing a garbage value.
func TestNextMinor_UnparseableReturnsUnchanged(t *testing.T) {
	cases := []string{"", "1.2", "1.2.3.4", "a.b.c", "v1.2.3"}
	for _, c := range cases {
		if got := NextMinor(c); got != c {
			t.Errorf("NextMinor(%q) = %q, want unchanged %q", c, got, c)
		}
	}
}

// TestCapabilities_AdvertisesDBDryRunSafe pins the capability a remote caller
// requires before forwarding --dry-run (P7-PROD-84), and that the returned
// slice is a copy: mutating it must not change what the build advertises.
func TestCapabilities_AdvertisesDBDryRunSafe(t *testing.T) {
	caps := Capabilities()
	found := false
	for _, c := range caps {
		if c == CapDBDryRunSafe {
			found = true
		}
	}
	if !found || CapDBDryRunSafe != "db-dry-run-safe" {
		t.Fatalf("Capabilities() = %v, want to include %q", caps, "db-dry-run-safe")
	}
	caps[0] = "tampered"
	if Capabilities()[0] != CapDBDryRunSafe {
		t.Error("Capabilities() shares state between calls")
	}
}
