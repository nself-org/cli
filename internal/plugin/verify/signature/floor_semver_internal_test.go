package signature

// floor_semver_internal_test.go — boundary tests for the floor's version parser and the pre-signing loader (D-1326).
//
// Purpose: pin the character-class edges of validPre, the leading-zero rule, isDigits and the loadPresigning error path, which the mutation run found unpinned.
// Inputs: literal strings on both sides of each edge.
// Outputs: test failures.
// Constraints: package-internal; no keys, no embedded data.

import "testing"

// TestValidPre_CharacterClassEdges: every edge of [0-9A-Za-z-] is valid, the
// ASCII neighbours just outside each edge are not.
func TestValidPre_CharacterClassEdges(t *testing.T) {
	for _, ok := range []string{"a0", "a9", "az", "aA", "aZ", "a-", "-", "rc.1", "x.0", "0", "10", "a.b.c"} {
		if !validPre(ok) {
			t.Errorf("validPre(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"a/", "a:", "a`", "a{", "a@", "a[", "a_", "a b", "", "a.", ".a", "a..b", "01", "00", "1.01", "a.00"} {
		if validPre(bad) {
			t.Errorf("validPre(%q) = true, want false", bad)
		}
	}
}

// TestIsDigits_Edges: '0' and '9' are digits, their ASCII neighbours are not.
func TestIsDigits_Edges(t *testing.T) {
	for _, s := range []string{"0", "9", "10", "09", "0123456789"} {
		if !isDigits(s) {
			t.Errorf("isDigits(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "/", ":", "1/", "1:", "a", "-1", "1.0"} {
		if isDigits(s) {
			t.Errorf("isDigits(%q) = true, want false", s)
		}
	}
}

// TestPreLess_NumericIdentifiers: numeric identifiers compare by length first,
// then as text, so "2" < "10" and "10" < "11", never the reverse.
func TestPreLess_NumericIdentifiers(t *testing.T) {
	for _, c := range []struct {
		a, b string
		less bool
	}{
		{"2", "10", true}, {"10", "2", false}, {"10", "11", true}, {"11", "10", false},
		{"9", "10", true}, {"0", "1", true}, {"1", "0", false}, {"1", "alpha", true}, {"alpha", "1", false},
		{"alpha", "beta", true}, {"beta", "alpha", false}, {"a", "a.1", true}, {"a.1", "a", false}, {"a", "a", false},
	} {
		if got := preLess(c.a, c.b); got != c.less {
			t.Errorf("preLess(%q, %q) = %v, want %v", c.a, c.b, got, c.less)
		}
	}
}

// TestLoadPresigning: a broken file is an error (UnsignedAllowed then refuses),
// a missing object is an empty list, a valid one decodes.
func TestLoadPresigning(t *testing.T) {
	if _, err := loadPresigning([]byte(`{"presigning": [`)); err == nil {
		t.Fatal("loadPresigning accepted broken JSON")
	}
	if m, err := loadPresigning([]byte(`{}`)); err != nil || len(m) != 0 {
		t.Fatalf("loadPresigning({}) = %v, %v; want empty, nil", m, err)
	}
	m, err := loadPresigning([]byte(`{"presigning":{"free":{"ci":[{"version":"1.0.0","sha256":"ab"}]}}}`))
	if err != nil || len(m["free"]["ci"]) != 1 || m["free"]["ci"][0].Version != "1.0.0" || m["free"]["ci"][0].SHA256 != "ab" {
		t.Fatalf("loadPresigning = %v, %v", m, err)
	}
}
