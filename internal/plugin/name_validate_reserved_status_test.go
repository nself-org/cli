package plugin

// Purpose: D-0545: "status" is reserved because the licensed nself-status-page
// plugin owns schema np_status; a plugin named "status" would map onto it.
// Constraints: no I/O.

import (
	"strings"
	"testing"
)

// TestStatusNameReservedButNeighboursPass pins the reservation of "status" and
// proves the neighbouring names stay usable.
func TestStatusNameReservedButNeighboursPass(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"status", true},
		{"status-page", false},
		{"nself-status-page", false},
		{"statuses", false},
		{"uptime-status", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateName(tc.name)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("ValidateName(%q) = %v, want nil", tc.name, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateName(%q) = nil, want a reserved-name error", tc.name)
			}
			for _, want := range []string{"reserved", "np_status"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("ValidateName(%q) error %q does not mention %q", tc.name, err.Error(), want)
				}
			}
		})
	}
}
