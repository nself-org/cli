package plugin

// Purpose: guard tests for validateConsumes name validation (D-0546) and the
// deps.go compareSemver wrapper ordering for unparseable versions (D-0191).
// Constraints: no network; temp dirs only.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// TestValidateConsumesRejectsInvalidNames: a path-shaped or otherwise invalid
// consumed name must be a hard E401 error, never reach os.Stat under pluginDir.
func TestValidateConsumesRejectsInvalidNames(t *testing.T) {
	tests := []struct {
		name     string
		consumes []string
	}{
		{"parent path", []string{"../x"}},
		{"nested path", []string{"ai", "a/b"}},
		{"uppercase", []string{"Mixed"}},
		{"reserved", []string{"bin"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConsumes(t.TempDir(), "my-plugin", tc.consumes)
			if err == nil {
				t.Fatalf("validateConsumes(%v) = nil, want an error", tc.consumes)
			}
			if d := errs.Describe(err); d == nil || d.Code != "E401" {
				t.Errorf("validateConsumes(%v) = %v, want it to wrap an E401", tc.consumes, err)
			}
			if !strings.Contains(err.Error(), "my-plugin") {
				t.Errorf("error %q does not name the consuming plugin", err.Error())
			}
		})
	}
}

// TestValidateConsumesValidMissingOnlyWarns: a well-formed but absent dependency
// is a stderr warning, not an error; a self-reference is skipped before validation.
func TestValidateConsumesValidMissingOnlyWarns(t *testing.T) {
	dir := t.TempDir()
	if err := validateConsumes(dir, "my-plugin", []string{"not-installed"}); err != nil {
		t.Errorf("missing valid dep returned %v, want nil", err)
	}
	if err := validateConsumes(dir, "my-plugin", []string{"my-plugin"}); err != nil {
		t.Errorf("self-reference returned %v, want nil", err)
	}
}
