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
