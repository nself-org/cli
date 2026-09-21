package ssl

// hosts_gate_test.go — proves the fix for the production-box incident: a
// bare `nself build` appended 14 *.local.nself.org entries to /etc/hosts on
// a live server. shouldManageHosts is the gate that must now refuse that
// shape of request outright.

import "testing"

// TestShouldManageHosts_ProdVetoesEverything is the direct regression test
// for the incident: ENV=prod must refuse /etc/hosts management no matter
// how local-dev-shaped the domain looks or whether --hosts was passed.
func TestShouldManageHosts_ProdVetoesEverything(t *testing.T) {
	cases := []struct {
		name          string
		baseDomain    string
		explicitHosts bool
	}{
		{"local-dev-shaped domain, no flag", "app.local.nself.org", false},
		{"local-dev-shaped domain, with flag", "app.local.nself.org", true},
		{"public domain, no flag", "nself.org", false},
		{"public domain, with flag", "nself.org", true},
		{"localhost, with flag", "localhost", true},
	}
	for _, c := range cases {
		if shouldManageHosts("prod", c.baseDomain, c.explicitHosts) {
			t.Errorf("%s: shouldManageHosts(prod, %q, %v) = true, want false (ENV=prod must veto unconditionally)",
				c.name, c.baseDomain, c.explicitHosts)
		}
	}
}

// TestShouldManageHosts_LocalDevDomainAllowedAutomatically verifies the
// normal local dev experience keeps working with zero flags in every
// non-prod environment.
func TestShouldManageHosts_LocalDevDomainAllowedAutomatically(t *testing.T) {
	domains := []string{
		"localhost",
		"api.local.nself.org",
		"admin.local.nself.org",
		"myapp.localhost",
		"myapp.local",
	}
	for _, env := range []string{"dev", "staging", ""} {
		for _, d := range domains {
			if !shouldManageHosts(env, d, false) {
				t.Errorf("shouldManageHosts(%q, %q, false) = false, want true (recognized local-dev domain)", env, d)
			}
		}
	}
}

// TestShouldManageHosts_UnrecognizedDomainNeedsExplicitFlag verifies a
// domain that doesn't match the local-dev shape is left alone by default,
// and only written when the operator explicitly opts in via --hosts — and
// only outside prod (covered separately above).
func TestShouldManageHosts_UnrecognizedDomainNeedsExplicitFlag(t *testing.T) {
	if shouldManageHosts("dev", "nself.org", false) {
		t.Error("shouldManageHosts(dev, nself.org, false) = true, want false (public domain, no explicit opt-in)")
	}
	if !shouldManageHosts("dev", "nself.org", true) {
		t.Error("shouldManageHosts(dev, nself.org, true) = false, want true (explicit --hosts opt-in, non-prod)")
	}
	if shouldManageHosts("staging", "custom.example.org", false) {
		t.Error("shouldManageHosts(staging, custom.example.org, false) = true, want false")
	}
}

// TestIsLocalDevDomain is table-driven over the exact suffix/equality rules,
// including near-miss strings that must NOT match (e.g. a domain that merely
// contains "localhost" as a substring without the "." separator).
func TestIsLocalDevDomain(t *testing.T) {
	cases := []struct {
		domain string
		want   bool
	}{
		{"localhost", true},
		{"LOCALHOST", true}, // case-insensitive
		{"api.local.nself.org", true},
		{"deeply.nested.local.nself.org", true},
		{"myapp.localhost", true},
		{"myapp.local", true},
		{"", false},
		{"nself.org", false},
		{"local.nself.org.evil.com", false}, // suffix trick: prefix match only, not suffix
		{"evil-localhost", false},           // no "." separator before "localhost"
		{"notlocal", false},
		{"custom.example.org", false},
	}
	for _, c := range cases {
		if got := isLocalDevDomain(c.domain); got != c.want {
			t.Errorf("isLocalDevDomain(%q) = %v, want %v", c.domain, got, c.want)
		}
	}
}

// TestCanWriteHostsFile_WritableFile verifies the permission pre-check
// succeeds for an ordinary writable file without modifying its contents.
func TestCanWriteHostsFile_WritableFile(t *testing.T) {
	path := tempHostsFile(t, "127.0.0.1\tlocalhost\n")

	if !canWriteHostsFile(path) {
		t.Errorf("canWriteHostsFile(%q) = false, want true for an ordinary writable file", path)
	}

	// Must not have modified the file's contents.
	lines, err := readHostsFile(path)
	if err != nil {
		t.Fatalf("readHostsFile after canWriteHostsFile: %v", err)
	}
	if len(lines) != 1 || lines[0] != "127.0.0.1\tlocalhost" {
		t.Errorf("canWriteHostsFile modified file contents: got %v", lines)
	}
}

// TestCanWriteHostsFile_MissingFile verifies the pre-check reports false
// (rather than panicking or treating it as writable) for a path that
// doesn't exist.
func TestCanWriteHostsFile_MissingFile(t *testing.T) {
	if canWriteHostsFile("/nonexistent-dir-for-nself-tests/hosts") {
		t.Error("canWriteHostsFile on a nonexistent path = true, want false")
	}
}
