package credmask

import (
	"strings"
	"testing"
	"time"
)

// bestOf returns the fastest of n runs of Schemeless over unit repeated to
// about size bytes; the minimum filters scheduler and GC noise on a loaded CI
// runner.
func bestOf(unit string, size, n int) time.Duration {
	v := strings.Repeat(unit, size/len(unit))
	best := time.Duration(1<<63 - 1)
	for i := 0; i < n; i++ {
		start := time.Now()
		_ = Schemeless(v)
		if d := time.Since(start); d < best {
			best = d
		}
	}
	return best
}

// TestSchemelessLinear: adversarial shapes must scale linearly. 10x the input
// may take about 10x as long; a quadratic scan takes about 100x, so the bound
// is 50x plus a 2 ms floor for timer noise. The 40 KB to 400 KB step is where
// a quadratic rescan stands clear of that noise.
func TestSchemelessLinear(t *testing.T) {
	units := []string{"@a(", ":@a(", "a:@", "a:b@c(", "x=y@a(", "password=", "aaaaaaaa", "::::", "@@@@"}
	for _, unit := range units {
		prev := bestOf(unit, 4_000, 7)
		for _, size := range []int{40_000, 400_000} {
			cur := bestOf(unit, size, 7)
			if cur > 50*prev+2*time.Millisecond {
				t.Errorf("unit %q: %d B took %v, %d B took %v: not linear", unit, size/10, prev, size, cur)
			}
			prev = cur
		}
	}
}
