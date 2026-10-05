package main

import (
	"context"
	"flag"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/tools/perfbench/scenarios"
)

// ABResult is the `ab -json` document: the run fields (metrics = head), the
// base and head metrics, the verdict and the failed metric names.
type ABResult struct {
	Result
	Base    abSide   `json:"base"`
	Head    abSide   `json:"head"`
	Verdict string   `json:"verdict"`
	Failed  []string `json:"failed"`
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

// abMeasure runs every probe of p interleaved (base, head, base, head, ...)
// so machine drift hits both binaries alike. Warm-up pairs are discarded.
func abMeasure(ctx context.Context, p scenarios.Prober, baseBin, headBin string, runs, warmup int, slowdown float64) (base, head []Metric, err error) {
	exits := scenarios.ExitTracker{}
	var bs, hs []scenarios.Sample
	for _, probe := range p.Probes() {
		for i := 0; i < warmup+runs; i++ {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			bms, bexit, err := scenarios.RunOnce(ctx, baseBin, nil, probe.Args, 0)
			if err != nil {
				return nil, nil, err
			}
			hms, hexit, err := scenarios.RunOnce(ctx, headBin, nil, probe.Args, slowdown)
			if err != nil {
				return nil, nil, err
			}
			if err := exits.Check("base:"+probe.Metric, bexit); err != nil {
				return nil, nil, err
			}
			if err := exits.Check("head:"+probe.Metric, hexit); err != nil {
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
	runs := fs.Int("runs", 40, "measured runs per probe and binary")
	warmup := fs.Int("warmup", 3, "discarded warm-up pairs per probe")
	ratio := fs.Float64("ratio", 1.25, "regression needs head p50 > ratio x base p50")
	minDelta := fs.Float64("min-delta-ms", 3, "and head p50 - base p50 > this many ms")
	slow := fs.Float64("inject-slowdown", 1.0, "SELF-TEST ONLY: make each head sample take F x its time")
	asJSON := fs.Bool("json", false, "print the ab JSON document")
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
	base, head, err := abMeasure(context.Background(), prober, *baseBin, *headBin, *runs, *warmup, *slow)
	if err != nil {
		sayln(stderr, "ab:", err)
		return 2
	}
	failed := compare(base, head, *ratio, *minDelta)
	verdict := "pass"
	if len(failed) > 0 {
		verdict = "fail"
	}
	if *asJSON {
		res := newResult(*scenario, *runs, head)
		res.Injected = *slow > 1
		out, err := marshal(ABResult{Result: res, Base: abSide{base}, Head: abSide{head}, Verdict: verdict, Failed: failed})
		if err != nil {
			sayln(stderr, "ab:", err)
			return 2
		}
		put(stdout, out)
	} else {
		say(stdout, "base:\n%shead:\n%sverdict: %s %s\n", formatMetrics(base), formatMetrics(head), verdict, strings.Join(failed, " "))
	}
	if len(failed) > 0 {
		return 1
	}
	return 0
}
