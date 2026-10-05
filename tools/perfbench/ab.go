package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/tools/perfbench/scenarios"
)

// G8 defaults (EPIC P7-GUARD decision G8). The ab flags default to these and
// the workflow relies on the defaults, so a test pins them.
const (
	DefaultRatio      = 1.25 // regression needs head p50 > ratio x base p50
	DefaultMinDeltaMS = 3.0  // and head p50 - base p50 > this many ms
	DefaultRuns       = 40   // interleaved rounds per probe
	DefaultWarmup     = 3    // discarded warm-up pairs per probe
)

// ABResult is the `ab -json` document: the run fields (metrics = head), the
// base and head metrics, the verdict and the failed metric names.
type ABResult struct {
	Result
	Base    abSide   `json:"base"`
	Head    abSide   `json:"head"`
	Verdict string   `json:"verdict"`
	Failed  []string `json:"failed"`
	Error   string   `json:"error,omitempty"` // set when a probe died, exited unexpectedly or timed out
}

type abSide struct {
	Metrics []Metric `json:"metrics"`
}

// regressed reports whether head is a regression against base per EPIC G8:
// head_p50 > ratio x base_p50 AND head_p50 - base_p50 > minDeltaMS. Both
// conditions are required, so a 2x slowdown of an 11 ms probe fires while
// timer noise on a tiny probe does not.
func regressed(base, head Metric, ratio, minDeltaMS float64) bool {
	return head.P50 > ratio*base.P50 && head.P50-base.P50 > minDeltaMS
}

// compare returns the metric names (in base order) that regressed. A metric
// present on one side only is reported as failed.
func compare(base, head []Metric, ratio, minDeltaMS float64) []string {
	byName := map[string]Metric{}
	for _, m := range head {
		byName[m.Name] = m
	}
	failed := []string{}
	for _, b := range base {
		h, ok := byName[b.Name]
		if !ok || regressed(b, h, ratio, minDeltaMS) {
			failed = append(failed, b.Name)
		}
	}
	return failed
}

// abMeasure runs every probe of p interleaved in rounds of one base and one
// head run, the order alternating each round, so machine drift hits both
// binaries alike. Warm-up pairs are discarded but
// still checked. A probe on either side that exits with a code other than the
// probe's expected one, dies by signal or times out returns a *ProbeFailure:
// a head that crashes at startup must never look faster.
func abMeasure(ctx context.Context, p scenarios.Prober, baseBin, headBin string, runs, warmup int, slowdown float64, timeout time.Duration) (base, head []Metric, err error) {
	var bs, hs []scenarios.Sample
	for _, probe := range p.Probes() {
		for i := 0; i < warmup+runs; i++ {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			// Alternate which binary goes first so a fixed order effect (the
			// second process finds a warmer cache, or a busier neighbour)
			// cannot favour either side.
			var bms, hms float64
			var err error
			if i%2 == 0 {
				if bms, err = scenarios.RunChecked(ctx, baseBin, "base", probe, scenarios.Opts{Timeout: timeout}); err == nil {
					hms, err = scenarios.RunChecked(ctx, headBin, "head", probe, scenarios.Opts{Timeout: timeout, Slowdown: slowdown})
				}
			} else {
				if hms, err = scenarios.RunChecked(ctx, headBin, "head", probe, scenarios.Opts{Timeout: timeout, Slowdown: slowdown}); err == nil {
					bms, err = scenarios.RunChecked(ctx, baseBin, "base", probe, scenarios.Opts{Timeout: timeout})
				}
			}
			if err != nil {
				return nil, nil, err
			}
			if i >= warmup {
				bs = append(bs, scenarios.Sample{Metric: probe.Metric, Unit: "ms", Value: bms})
				hs = append(hs, scenarios.Sample{Metric: probe.Metric, Unit: "ms", Value: hms})
			}
		}
	}
	if base, err = groupSamples(bs); err != nil {
		return nil, nil, err
	}
	head, err = groupSamples(hs)
	return base, head, err
}

// abCmd implements `perfbench ab -base <bin> -head <bin> [flags]`. Exit 0 on
// pass, 1 on a regression, 2 on a usage or measurement error.
func abCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ab", flag.ContinueOnError)
	fs.SetOutput(stderr)
	scenario := fs.String("scenario", "cold-start", "scenario name; must be made of fixed probes")
	baseBin := fs.String("base", "", "base nself binary")
	headBin := fs.String("head", "", "head nself binary")
	runs := fs.Int("runs", DefaultRuns, "measured runs per probe and binary")
	warmup := fs.Int("warmup", DefaultWarmup, "discarded warm-up pairs per probe")
	ratio := fs.Float64("ratio", DefaultRatio, "regression needs head p50 > ratio x base p50")
	minDelta := fs.Float64("min-delta-ms", DefaultMinDeltaMS, "and head p50 - base p50 > this many ms")
	slow := fs.Float64("inject-slowdown", 1.0, "SELF-TEST ONLY: make each head sample take F x its time")
	asJSON := fs.Bool("json", false, "print the ab JSON document")
	headSHA := fs.String("head-sha", "", "revision of the head binary, recorded in the result (default: empty)")
	timeout := fs.Duration("timeout", scenarios.DefaultTimeout, "kill and fail one probe run that takes longer")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *baseBin == "" || *headBin == "" || *runs < 1 || *warmup < 0 || *slow < 1 {
		sayln(stderr, "ab: -base and -head are required, -runs >= 1, -warmup >= 0, -inject-slowdown >= 1")
		return 2
	}
	for _, b := range []string{*baseBin, *headBin} {
		if _, err := os.Stat(b); err != nil {
			sayln(stderr, "ab:", err)
			return 2
		}
	}
	sc, ok := scenarios.Get(*scenario)
	prober, isProber := sc.(scenarios.Prober)
	if !ok || !isProber {
		say(stderr, "ab: scenario %q is unknown or has no fixed probes (have: %s)\n", *scenario, strings.Join(scenarios.Names(), ", "))
		return 2
	}
	base, head, err := abMeasure(context.Background(), prober, *baseBin, *headBin, *runs, *warmup, *slow, *timeout)
	var pf *scenarios.ProbeFailure
	if errors.As(err, &pf) {
		// A dead, wrong-exit or hung binary fails the verdict (exit 1), like a regression.
		sayln(stderr, "ab:", err)
		if *asJSON {
			res := newResult(*scenario, *headSHA, *runs, []Metric{})
			res.Injected = *slow > 1
			if !emitAB(stdout, stderr, ABResult{Result: res, Base: abSide{[]Metric{}}, Head: abSide{[]Metric{}},
				Verdict: "fail", Failed: []string{pf.Metric}, Error: err.Error()}) {
				return 2
			}
		}
		return 1
	}
	if err != nil {
		sayln(stderr, "ab:", err)
		return 2
	}
	if err := validateMetrics(base, head); err != nil {
		sayln(stderr, "ab:", err, "; refusing to pass")
		return 2
	}
	failed := compare(base, head, *ratio, *minDelta)
	verdict := "pass"
	if len(failed) > 0 {
		verdict = "fail"
	}
	if *asJSON {
		res := newResult(*scenario, *headSHA, *runs, head)
		res.Injected = *slow > 1
		if !emitAB(stdout, stderr, ABResult{Result: res, Base: abSide{base}, Head: abSide{head}, Verdict: verdict, Failed: failed}) {
			return 2
		}
	} else {
		say(stdout, "base:\n%shead:\n%sverdict: %s %s\n", formatMetrics(base), formatMetrics(head), verdict, strings.Join(failed, " "))
	}
	if len(failed) > 0 {
		return 1
	}
	return 0
}

// validateMetrics refuses an empty metric set and any NaN or infinite value:
// NaN compares as "not regressed" and cannot be marshalled, so it must never
// reach the verdict.
func validateMetrics(base, head []Metric) error {
	if len(base) == 0 || len(head) == 0 {
		return errors.New("the scenario produced no metrics")
	}
	for _, side := range []struct {
		name string
		ms   []Metric
	}{{"base", base}, {"head", head}} {
		for _, m := range side.ms {
			for _, v := range []float64{m.P50, m.P95, m.Max} {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return fmt.Errorf("%s metric %s has a non-finite value", side.name, m.Name)
				}
			}
		}
	}
	return nil
}

// emitAB prints the ab JSON document. It reports false (the caller exits 2)
// when the document cannot be marshalled, so an empty perf-ab.json never
// stands in for a verdict.
func emitAB(stdout, stderr io.Writer, r ABResult) bool {
	out, err := marshal(r)
	if err != nil {
		sayln(stderr, "ab: cannot write the JSON result:", err)
		return false
	}
	put(stdout, out)
	return true
}
