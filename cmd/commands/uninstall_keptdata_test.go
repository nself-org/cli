package commands

// Purpose: unit test for uninstallKeptData (D-0375): data counts as kept only
// when compose down --volumes did not run and .nself/volumes/ was not removed.

import "testing"

func TestUninstallKeptData(t *testing.T) {
	cases := []struct {
		name           string
		volumesDeleted bool
		removed        []string
		want           bool
	}{
		{"nothing deleted", false, []string{"docker-compose.yml"}, true},
		{"compose down --volumes ran", true, nil, false},
		{"volumes dir removed, no compose", false, []string{".nself/cache/", ".nself/volumes/"}, false},
		{"only cache removed", false, []string{".nself/cache/"}, true},
		{"volumes dir removal failed (not listed)", false, []string{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := uninstallKeptData(tc.volumesDeleted, tc.removed); got != tc.want {
				t.Fatalf("uninstallKeptData = %v, want %v", got, tc.want)
			}
		})
	}
}
