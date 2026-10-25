package pluginname

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestValidate(t *testing.T) {
	for _, ok := range []string{"notify", "ai", "a-b-c", "x1", strings.Repeat("a", MaxLen)} {
		if err := Validate(ok); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "../x", "a/b", `a\b`, "/abs", "-x", "a\x00b", "a b", "Notify", "notify.prev", "a_b", "1a",
		"bin", "common", "status", strings.Repeat("a", MaxLen+1)} {
		err := Validate(bad)
		if d := errs.Describe(err); d == nil || d.Code != "E401" {
			t.Errorf("Validate(%q) = %v, want E401", bad, err)
		}
	}
}

func TestSlugReasonAllowsReservedNames(t *testing.T) {
	// Reserved names are plain slugs: only Validate refuses them, so
	// ValidateCommandName (which uses SlugReason) can still accept "status".
	for name := range Reserved {
		if r := SlugReason(name); r != "" {
			t.Errorf("SlugReason(%q) = %q, want empty", name, r)
		}
	}
}
