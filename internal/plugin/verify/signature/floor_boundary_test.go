package signature

import "testing"

// TestValidPreBoundaries pins every edge of the pre-release character classes and the leading-zero
// rule: each accepted class at both ends, and the character just outside each end.
func TestValidPreBoundaries(t *testing.T) {
	for _, ok := range []string{"a", "z", "A", "Z", "0", "9", "-", "a0", "a9", "az", "aA", "aZ", "a-", "0a", "10", "alpha.1", "x-y.0"} {
		if !validPre(ok) {
			t.Errorf("validPre(%q) = false, want true", ok)
		}
	}
	// Neighbours of each range end: '/' ':' before and after the digits, '@' '[' around A-Z, '`' '{' around a-z.
	for _, bad := range []string{"a/", "a:", "a@", "a[", "a`", "a{", "/", ":", "@", "[", "`", "{", "", "a.", ".a", "a..b", "01", "00", "a.01", "x.00"} {
		if validPre(bad) {
			t.Errorf("validPre(%q) = true, want false", bad)
		}
	}
	if !validPre("0") || validPre("00") || !validPre("a0") || !validPre("10") {
		t.Error("a lone zero is valid, a leading zero on a longer number is not")
	}
}

func TestIsDigitsBoundaries(t *testing.T) {
	for _, ok := range []string{"0", "9", "09", "90", "1234567890"} {
		if !isDigits(ok) {
			t.Errorf("isDigits(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", "/", ":", "a", "1a", "a1", "1.2", "-1"} {
		if isDigits(bad) {
			t.Errorf("isDigits(%q) = true", bad)
		}
	}
}

// TestLoadPresigning: a decodable file yields its entries; a broken one is an error, so
// UnsignedAllowed fails closed.
func TestLoadPresigning(t *testing.T) {
	m, err := loadPresigning([]byte(`{"presigning":{"free":{"ci":[{"version":"1.0.0","sha256":"ab"}]}}}`))
	if err != nil || len(m["free"]["ci"]) != 1 || m["free"]["ci"][0].Version != "1.0.0" || m["free"]["ci"][0].SHA256 != "ab" {
		t.Fatalf("loadPresigning = %v, %v", m, err)
	}
	if m, err = loadPresigning([]byte(`{}`)); err != nil || len(m) != 0 {
		t.Fatalf("no presigning object must be empty, got %v, %v", m, err)
	}
	if _, err = loadPresigning([]byte(`{"presigning": [`)); err == nil {
		t.Fatal("loadPresigning accepted broken JSON")
	}
}
