package remote

import "testing"

// Pin the reserved extra pair before append growth conceals arithmetic errors.
func TestCIOptionsBaseAllocation(t *testing.T) {
	for _, minor := range []int{4, 5, 6} {
		out := CIOptions("node", "/pins/known_hosts", Version{8, minor})
		want := 40
		if minor >= 5 {
			want = 42
		}
		if len(out) != want || cap(out) != 42 {
			t.Errorf("8.%d: len/cap=%d/%d, want %d/42", minor, len(out), cap(out), want)
		}
	}
}
