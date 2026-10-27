package credmask

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// sink keeps the compiler from discarding the measured call.
var sink string

// shapes are the adversarial inputs: each is repeated to the test size. They
// hit the DSN token scan (`@a(` runs with and without a colon or `://` ahead),
// the bare user:pass@host rule and the keyword rule, plus plain runs.
var shapes = []struct{ name, prefix, unit string }{
	{"@a(", "", "@a("},
	{":@a(", "", ":@a("},
	{"a:@", "", "a:@"},
	{"a:b@c(", "", "a:b@c("},
	{"x=y@a(", "", "x=y@a("},
	{"password=", "", "password="},
	{"password= x", "", "password= x"},
	{"aaaaaaaa", "", "aaaaaaaa"},
	{"::::", "", "::::"},
	{"@@@@", "", "@@@@"},
	{"one colon then @a(", "a:", "@a("},
	{"scheme then @a(", "a://h:", "@a("},
	{"quote then @a(", "'a:", "@a("},
}

func build(prefix, unit string, size int) string {
	return prefix + strings.Repeat(unit, size/len(unit))
}

// work is what one Schemeless call costs, measured without a clock.
type work struct{ steps, mallocs, bytes uint64 }

// measure runs Schemeless once over v and returns the bytes its scans read
// (stepHook), plus the allocation count and bytes it caused. The allocation
// figures are the smallest of three runs, so a stray allocation by another
// goroutine does not inflate them.
func measure(v string) work {
	var w work
	stepHook = func(n int) { w.steps += uint64(n) }
	sink = Schemeless(v)
	stepHook = nil
	w.mallocs, w.bytes = ^uint64(0), ^uint64(0)
	for i := 0; i < 3; i++ {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		sink = Schemeless(v)
		runtime.ReadMemStats(&after)
		w.mallocs = min(w.mallocs, after.Mallocs-before.Mallocs)
		w.bytes = min(w.bytes, after.TotalAlloc-before.TotalAlloc)
	}
	return w
}

// growthLimit is the most a 10x input may grow any cost: linear is 10x, a
// quadratic scan or per-candidate string rebuild is 100x.
const growthLimit = 15

// bytesGrowthLimit is looser for allocated bytes: slice doubling and size
// classes make 10x input read as up to about 18x memory at 4 KB to 40 KB, while
// bytes per input byte is flat from 256 KB up. A quadratic rebuild is 100x.
const bytesGrowthLimit = 25

// TestSchemelessLinear proves linear scaling without timing, so it holds on
// every platform: for each adversarial shape, 10x the input (4 KB to 40 KB)
// must read under 15x the bytes, make under 15x the allocations and allocate
// under 25x the memory. The small additive slack covers fixed costs (regexp
// state, builder growth) that dominate tiny inputs. Under -race only the step
// check runs (see raceEnabled).
func TestSchemelessLinear(t *testing.T) {
	for _, s := range shapes {
		small, large := measure(build(s.prefix, s.unit, 4_000)), measure(build(s.prefix, s.unit, 40_000))
		t.Logf("%-20s steps %8d -> %9d (x%.1f)  mallocs %6d -> %7d (x%.1f)  bytes %8d -> %9d (x%.1f)", s.name,
			small.steps, large.steps, ratio(small.steps, large.steps),
			small.mallocs, large.mallocs, ratio(small.mallocs, large.mallocs),
			small.bytes, large.bytes, ratio(small.bytes, large.bytes))
		if large.steps > growthLimit*small.steps+64 {
			t.Errorf("%q: 4 KB read %d bytes, 40 KB read %d: not linear", s.name, small.steps, large.steps)
		}
		if raceEnabled {
			continue
		}
		if large.mallocs > growthLimit*small.mallocs+16 {
			t.Errorf("%q: 4 KB made %d allocations, 40 KB made %d: not linear", s.name, small.mallocs, large.mallocs)
		}
		if large.bytes > bytesGrowthLimit*small.bytes+4096 {
			t.Errorf("%q: 4 KB allocated %d bytes, 40 KB allocated %d: not linear", s.name, small.bytes, large.bytes)
		}
	}
}

func ratio(a, b uint64) float64 {
	if a == 0 {
		return 0
	}
	return float64(b) / float64(a)
}

// TestSchemelessLinearWallClock is a coarse safety net for a slowdown the
// step hook cannot see (a loop added without it). It is skipped on Windows,
// whose timer and shared runners are too coarse and noisy for a ratio. It takes
// the best of five batches per size and allows 40x for 10x the input plus a
// floor of a few milliseconds; a quadratic scan costs about 100x.
func TestSchemelessLinearWallClock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wall-clock ratios are unreliable on Windows; TestSchemelessLinear covers it")
	}
	if testing.Short() {
		t.Skip("wall-clock safety net skipped in -short")
	}
	best := func(v string) time.Duration {
		d := time.Duration(1<<63 - 1)
		for i := 0; i < 5; i++ {
			start := time.Now()
			sink = Schemeless(v)
			d = min(d, time.Since(start))
		}
		return d
	}
	for _, s := range shapes {
		small, large := best(build(s.prefix, s.unit, 40_000)), best(build(s.prefix, s.unit, 400_000))
		if large > 40*small+5*time.Millisecond {
			t.Errorf("%q: 40 KB took %v, 400 KB took %v: not linear", s.name, small, large)
		}
	}
}
