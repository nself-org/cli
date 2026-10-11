package remote

import (
	"strings"
	"testing"
)

// Mutation-killing tests (D-0253): exact boundaries of the validators.

const (
	aliasExtra  = "._@:+-"
	nodeIDExtra = "._-"
)

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// TestAliasByte_EveryByte pins the accepted set across all 256 byte values.
func TestAliasByte_EveryByte(t *testing.T) {
	for i := 0; i < 256; i++ {
		c := byte(i)
		want := isAlnum(c) || strings.IndexByte(aliasExtra, c) >= 0
		if got := aliasByte(c); got != want {
			t.Errorf("aliasByte(%#x) = %v, want %v", c, got, want)
		}
	}
}

// TestValidateNodeID_EveryByte pins the accepted set inside a node id (after a
// safe first byte) across all 256 byte values.
func TestValidateNodeID_EveryByte(t *testing.T) {
	for i := 0; i < 256; i++ {
		c := byte(i)
		want := isAlnum(c) || strings.IndexByte(nodeIDExtra, c) >= 0
		if got := ValidateNodeID("x"+string([]byte{c})) == nil; got != want {
			t.Errorf("ValidateNodeID(x%#x) ok = %v, want %v", c, got, want)
		}
	}
}

func TestValidateAlias_LengthBoundary(t *testing.T) {
	if err := ValidateAlias(strings.Repeat("a", maxHostToken)); err != nil {
		t.Errorf("%d bytes refused: %v", maxHostToken, err)
	}
	if ValidateAlias(strings.Repeat("a", maxHostToken+1)) == nil {
		t.Errorf("%d bytes accepted", maxHostToken+1)
	}
}

func TestValidateNodeID_LengthBoundary(t *testing.T) {
	if err := ValidateNodeID(strings.Repeat("a", 128)); err != nil {
		t.Errorf("128 bytes refused: %v", err)
	}
	if ValidateNodeID(strings.Repeat("a", 129)) == nil {
		t.Error("129 bytes accepted")
	}
	if err := ValidateNodeID("a"); err != nil {
		t.Errorf("1 byte refused: %v", err)
	}
}

func TestSplitDest(t *testing.T) {
	for _, c := range []struct{ in, user, host string }{
		{"@host", "", "host"}, // '@' at index 0 still splits
		{"u@host", "u", "host"},
		{"a@b@host", "a@b", "host"}, // the last '@' wins
		{"host", "", "host"},
	} {
		if u, h := splitDest(c.in); u != c.user || h != c.host {
			t.Errorf("splitDest(%q) = (%q, %q), want (%q, %q)", c.in, u, h, c.user, c.host)
		}
	}
	if got := scpOperand("@host", "/p"); got != "@host:/p" {
		t.Errorf("scpOperand(@host) = %q", got)
	}
}

func TestValidateLegacyDest(t *testing.T) {
	ok := []string{"h", "host", "user@host", "u@203.0.113.7", "a!b", "~", "host:path", "\xc3\xa9"}
	for _, d := range ok {
		if err := ValidateLegacyDest(d); err != nil {
			t.Errorf("ValidateLegacyDest(%q) = %v", d, err)
		}
	}
	bad := []string{"", "-", "-oProxyCommand=x", "a b", " a", "a ", "a\tb", "a\nb", "a\x00b", "a\x01b", "a\x1fb", "a\x7fb"}
	for _, d := range bad {
		if ValidateLegacyDest(d) == nil {
			t.Errorf("ValidateLegacyDest(%q) = nil, want error", d)
		}
	}
	// every byte: refused exactly when <= 0x20 or 0x7f
	for i := 0; i < 256; i++ {
		c := byte(i)
		if i == '-' {
			continue
		}
		want := c <= ' ' || c == 0x7f
		got := ValidateLegacyDest("a"+string([]byte{c})+"b") != nil
		if i >= 0x80 {
			want = false
		}
		if got != want {
			t.Errorf("ValidateLegacyDest(a%#xb) refused = %v, want %v", c, got, want)
		}
	}
}
