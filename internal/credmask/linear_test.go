package credmask

import (
	"strings"
	"testing"
	"time"
)

// sink keeps the compiler from discarding the measured call.
var sink string

// minBatch is how long one timed batch must run. Windows timers tick at about
// 0.5 to 15 ms, so a single 4 KB call reads as 0s there; a batch this long
// reads as a real number on every platform.
const minBatch = 3 * time.Millisecond

// perCall returns the cost of one Schemeless call over unit repeated to about
// size bytes. It doubles the batch until the batch lasts minBatch, then takes
// the fastest of n batches, so a scheduler or GC stall on a loaded runner does
// not read as super-linear growth and a zero reading never reaches a ratio.
func perCall(unit string, size, n int) time.Duration {
	v := strings.Repeat(unit, size/len(unit))
	run := func(reps int) time.Duration {
		start := time.Now()
		for i := 0; i < reps; i++ {
			sink = Schemeless(v)
		}
		return time.Since(start)
	}
	reps := 1
	for run(reps) < minBatch && reps < 1<<20 {
		reps *= 2
	}
	best := time.Duration(1<<63 - 1)
	for i := 0; i < n; i++ {
		if d := run(reps) / time.Duration(reps); d < best {
			best = d
		}
	}
	return best
}

// TestSchemelessLinear: adversarial shapes must scale linearly. 10x the input
// may take about 10x as long; a quadratic scan takes about 100x. The bound is
// 40x plus a 200 µs per-call floor for noise, so a 10x blow-up to 100x still
// fails. Costs are per call over batches of at least minBatch, never a single
// call, so a coarse timer cannot make the small case read as 0s.
func TestSchemelessLinear(t *testing.T) {
	units := []string{"@a(", ":@a(", "a:@", "a:b@c(", "x=y@a(", "password=", "aaaaaaaa", "::::", "@@@@"}
	for _, unit := range units {
		prev := perCall(unit, 4_000, 5)
		for _, size := range []int{40_000, 400_000} {
			cur := perCall(unit, size, 5)
			if cur > 40*prev+200*time.Microsecond {
				t.Errorf("unit %q: %d B took %v/call, %d B took %v/call: not linear", unit, size/10, prev, size, cur)
			}
			prev = cur
		}
	}
}
