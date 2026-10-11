package dockertest

import (
	"errors"
	"testing"
)

func TestStrict(t *testing.T) {
	for _, tc := range []struct {
		goos, ga string
		want     bool
	}{
		{"linux", "true", true},
		{"linux", "", false},  // cam CI host (CI=1, no GITHUB_ACTIONS)
		{"linux", "1", false}, // only the exact value GitHub sets counts
		{"darwin", "true", false},
		{"windows", "true", false},
	} {
		if got := Strict(tc.goos, tc.ga); got != tc.want {
			t.Errorf("Strict(%q, %q) = %v, want %v", tc.goos, tc.ga, got, tc.want)
		}
	}
}

func TestReason(t *testing.T) {
	for _, tc := range []struct {
		name   string
		out    string
		err    error
		usable bool
	}{
		{"linux daemon", "linux", nil, true},
		{"socket permission denied", "permission denied while trying to connect", errors.New("exit status 1"), false},
		{"windows containers", "windows", nil, false},
		{"empty answer", "", nil, false},
	} {
		if got := reason(tc.out, tc.err); (got == "") != tc.usable {
			t.Errorf("%s: reason = %q, want usable=%v", tc.name, got, tc.usable)
		}
	}
}
