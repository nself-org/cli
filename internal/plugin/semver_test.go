package plugin

// Purpose: table tests for CompareSemver (D-0191) covering the cases of both
// former implementations (bundle resolver and deps.go) plus the edge cases they
// disagreed on, and for the deps.go compareSemver wrapper's unparseable ordering.
// Constraints: pure functions, no I/O.

import "testing"

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		// From the bundle resolver tests.
		{"1.2.0", "1.1.0", 1},
		{"1.1.0", "1.2.0", -1},
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.9.9", 1},
		{"1.0.0", "1.0.0-beta", 1},
		{"1.0.0-alpha", "1.0.0", -1},
		{"v1.0.0", "1.0.0", 0},
		// From the plugin compat tests (CompareVersions).
		{"1.0.9", "1.0.9", 0},
		{"1.0.9", "1.0.5", 1},
		// Shapes and ordering.
		{"1.2", "1.2.0", 0},
		{"1", "1.0.0", 0},
		{"v1.2.3", "1.2.3", 0},
		{" 1.2.3 ", "1.2.3", 0},
		{"1.2.3-beta", "1.2.3", -1},
		{"1.2.3", "1.2.3-beta", 1},
		{"1.2.3-alpha", "1.2.3-beta", -1},
		{"1.2.3-beta", "1.2.3-beta", 0},
		{"1.10.0", "1.9.9", 1},
		{"1.9.9", "1.10.0", -1},
		{"0.2.3", "v1.2.3", -1},
		{"2.0.0-rc.1", "1.9.9", 1},
		// Build metadata never affects order.
		{"1.2.3+b1", "1.2.3", 0},
		{"1.2.3+b1", "1.2.3+b2", 0},
		{"1.2.3-rc.1+b2", "1.2.3-rc.1", 0},
		{"1.2.3-rc.1+b2", "1.2.3", -1},
		{"2.0.0+b1", "1.0.0", 1},
		{"1.0.0", "2.0.0+b1", -1},
		{"1.2.3+", "1.2.3", 0},
	}
	for _, tc := range tests {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			got, err := CompareSemver(tc.a, tc.b)
			if err != nil {
				t.Fatalf("CompareSemver(%q, %q) error: %v", tc.a, tc.b, err)
			}
			if got != tc.want {
				t.Errorf("CompareSemver(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestCompareSemverErrors(t *testing.T) {
	tests := []struct{ a, b string }{
		{"", "1.0.0"},
		{"1.0.0", ""},
		{"abc", "1.0.0"},
		{"1.0.0", "1.x"},
		{"1.x", "1.x"},
		{"1.2.3.4", "1.0.0"},
		{"+b", "1.0.0"},
		{"v", "1.0.0"},
	}
	for _, tc := range tests {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			if got, err := CompareSemver(tc.a, tc.b); err == nil {
				t.Errorf("CompareSemver(%q, %q) = %d, nil; want an error", tc.a, tc.b, got)
			}
		})
	}
}

// TestDepsCompareSemverWrapper pins the never-failing wrapper: unparseable
// versions sort below parseable ones and equal each other.
func TestDepsCompareSemverWrapper(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.2", 1},
		{"v1.2.3", "1.2.3", 0},
		{"1.2", "1.2.0", 0},
		{"abc", "0.0.1", -1},
		{"0.0.1", "abc", 1},
		{"", "1.0.0", -1},
		{"1.0.0", "", 1},
		{"abc", "xyz", 0},
		{"", "", 0},
		{"1.x", "", 0},
		{"2.0.0+b1", "1.0.0", 1},
		{"1.0.0", "2.0.0+b1", -1},
	}
	for _, tc := range tests {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			if got := compareSemver(tc.a, tc.b); got != tc.want {
				t.Errorf("compareSemver(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
			if got := CompareVersions(tc.a, tc.b); got != tc.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
