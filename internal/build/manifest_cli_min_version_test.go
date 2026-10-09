package build

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
)

func TestValidateCLIMinVersion(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"1.4.0", true}, {"v1.4.0", true}, {"0.0.1", true},
		{"1.4.0-rc.1+build.2", true}, {"soon", false}, {"1.4", false},
		{"", false}, {"01.2.3", false}, {"1.2.3-01", false},
	} {
		if got := ValidateCLIMinVersion(tc.value); got != tc.valid {
			t.Errorf("%q: valid=%v, want %v", tc.value, got, tc.valid)
		}
	}
	for _, tc := range []struct{ value, code string }{{"1.4.0", ""}, {"soon", CodeCLIMinVersion}} {
		findings := ValidateManifestBytes("nself.yaml", []byte("app: test\ncli_min_version: "+tc.value+"\n"))
		if tc.code == "" && len(findings) != 0 {
			t.Fatalf("valid value %q: %+v", tc.value, findings)
		}
		if tc.code != "" {
			if len(findings) != 1 || findings[0].Code != tc.code || findings[0].Path != "cli_min_version" {
				t.Fatalf("invalid value %q: %+v", tc.value, findings)
			}
			want := SeverityWarning
			if compat.V15() {
				want = SeverityError
			}
			if findings[0].Severity != want || !strings.Contains(findings[0].String(), "[E061]") {
				t.Fatalf("severity or rendering: %+v", findings[0])
			}
		}
	}
}
