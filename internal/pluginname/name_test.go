package pluginname

import (
	"strings"
	"testing"
)

func TestReason(t *testing.T) {
	for _, ok := range []string{"notify", "ai", "a-b-c", "x1", strings.Repeat("a", MaxLen)} {
		if r := Reason(ok); r != "" {
			t.Errorf("Reason(%q) = %q, want empty", ok, r)
		}
	}
	for _, bad := range []string{"", ".", "..", "../x", "a/b", `a\b`, "/abs", "-x", "a\x00b", "a b", "Notify", "notify.prev", "a_b", "1a",
		"bin", "common", "status", strings.Repeat("a", MaxLen+1)} {
		if Reason(bad) == "" {
			t.Errorf("Reason(%q) is empty, want a reason", bad)
		}
	}
}

func TestSlugReasonAllowsReservedNames(t *testing.T) {
	// Reserved names are plain slugs: only Reason refuses them, so
	// ValidateCommandName (which uses SlugReason) can still accept "status".
	for name := range Reserved {
		if r := SlugReason(name); r != "" {
			t.Errorf("SlugReason(%q) = %q, want empty", name, r)
		}
	}
}

func TestBinaryReason(t *testing.T) {
	for _, ok := range []string{"nself-foo", "tool.v2", "a_b", "Tool", strings.Repeat("a", MaxLen)} {
		if r := BinaryReason(ok); r != "" {
			t.Errorf("BinaryReason(%q) = %q, want empty", ok, r)
		}
	}
	for _, bad := range []string{"", ".", "..", "../x", "a/b", `a\b`, "/abs", "-x", ".hidden", "a\x00b", "a\nb", "a b", strings.Repeat("a", MaxLen+1)} {
		if BinaryReason(bad) == "" {
			t.Errorf("BinaryReason(%q) is empty, want a reason", bad)
		}
	}
}

func TestIsCommandBinary(t *testing.T) {
	for _, ok := range []string{"nself-a", "nself-foo-2"} {
		if !IsCommandBinary(ok) {
			t.Errorf("IsCommandBinary(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "nself-", "nself-2x", "nself-Foo", "nself-a_b", "nself-a.b", "nself-a\n", "foo-bar", "nself-../x"} {
		if IsCommandBinary(bad) {
			t.Errorf("IsCommandBinary(%q) = true, want false", bad)
		}
	}
}
