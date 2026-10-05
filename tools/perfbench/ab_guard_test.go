package main

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/nself-org/cli/tools/perfbench/scenarios"
)

// The G8 values as written in EPIC P7-GUARD and Benchmarks.md. Literals on
// purpose: changing a default must fail here until the epic changes.
func TestABDefaultsArePinnedToG8(t *testing.T) {
	if DefaultRatio != 1.25 || DefaultMinDeltaMS != 3 || DefaultRuns != 40 || DefaultWarmup != 3 {
		t.Fatalf("G8 constants drifted: ratio %v min-delta %v runs %v warmup %v",
			DefaultRatio, DefaultMinDeltaMS, DefaultRuns, DefaultWarmup)
	}
	// The flags must actually use the constants: -h prints "(default X)".
	var out, errb bytes.Buffer
	dispatch([]string{"ab", "-h"}, &out, &errb)
	usage := errb.String()
	for _, want := range []string{
		fmt.Sprintf("-ratio float\n    \tregression needs head p50 > ratio x base p50 (default %v)", DefaultRatio),
		fmt.Sprintf("-min-delta-ms float\n    \tand head p50 - base p50 > this many ms (default %v)", DefaultMinDeltaMS),
		fmt.Sprintf("-runs int\n    \tmeasured runs per probe and binary (default %v)", DefaultRuns),
		fmt.Sprintf("-warmup int\n    \tdiscarded warm-up pairs per probe (default %v)", DefaultWarmup),
	} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage lacks %q\n%s", want, usage)
		}
	}
	// Behavioural pin: with default thresholds 1.24x passes and 1.26x fails.
	base := Metric{P50: 100}
	if regressed(base, Metric{P50: 124}, DefaultRatio, DefaultMinDeltaMS) {
		t.Error("1.24x must pass at the default ratio")
	}
	if !regressed(base, Metric{P50: 126}, DefaultRatio, DefaultMinDeltaMS) {
		t.Error("1.26x must fail at the default ratio")
	}
	if regressed(Metric{P50: 1}, Metric{P50: 3.9}, DefaultRatio, DefaultMinDeltaMS) {
		t.Error("+2.9 ms must pass at the default floor")
	}
	if !regressed(Metric{P50: 1}, Metric{P50: 4.1}, DefaultRatio, DefaultMinDeltaMS) {
		t.Error("+3.1 ms must fail at the default floor")
	}
}

func TestValidateMetrics(t *testing.T) {
	ok := []Metric{{Name: "a", P50: 1, P95: 2, Max: 3}}
	if err := validateMetrics(ok, ok); err != nil {
		t.Fatalf("finite metrics: %v", err)
	}
	if err := validateMetrics(nil, ok); err == nil || !strings.Contains(err.Error(), "no metrics") {
		t.Errorf("empty base: %v", err)
	}
	if err := validateMetrics(ok, []Metric{}); err == nil || !strings.Contains(err.Error(), "no metrics") {
		t.Errorf("empty head: %v", err)
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, field := range []string{"p50", "p95", "max"} {
			m := Metric{Name: "a", P50: 1, P95: 2, Max: 3}
			switch field {
			case "p50":
				m.P50 = bad
			case "p95":
				m.P95 = bad
			case "max":
				m.Max = bad
			}
			if err := validateMetrics(ok, []Metric{m}); err == nil || !strings.Contains(err.Error(), "non-finite") {
				t.Errorf("head %s=%v: %v", field, bad, err)
			}
			if err := validateMetrics([]Metric{m}, ok); err == nil || !strings.Contains(err.Error(), "non-finite") {
				t.Errorf("base %s=%v: %v", field, bad, err)
			}
		}
	}
}

// emitAB cannot marshal a NaN; it must say so and report failure.
func TestEmitABFailureIsReported(t *testing.T) {
	var out, errb bytes.Buffer
	r := ABResult{Verdict: "pass", Failed: []string{}, Head: abSide{[]Metric{{Name: "a", P50: math.NaN()}}}}
	if emitAB(&out, &errb, r) {
		t.Error("NaN must make emitAB fail")
	}
	if out.Len() != 0 || !strings.Contains(errb.String(), "cannot write the JSON result") {
		t.Errorf("out %q err %q", out.String(), errb.String())
	}
	out.Reset()
	errb.Reset()
	r.Head = abSide{[]Metric{{Name: "a", P50: 1}}}
	if !emitAB(&out, &errb, r) || out.Len() == 0 {
		t.Errorf("finite result must be written: %q", errb.String())
	}
}

// emptyProbes is a scenario with fixed probes but none defined: the run
// produces no metrics, which must exit 2, never pass.
type emptyProbes struct{}

func (emptyProbes) Name() string { return "test-empty-probes" }
func (emptyProbes) Run(context.Context, scenarios.Config) ([]scenarios.Sample, error) {
	return nil, nil
}
func (emptyProbes) Probes() []scenarios.Probe { return nil }

func TestABRefusesEmptyMetrics(t *testing.T) {
	scenarios.Register(emptyProbes{})
	bin := script(t, "nself", okScript)
	for _, extra := range [][]string{{}, {"-json"}} {
		var out, errb bytes.Buffer
		args := append([]string{"ab", "-scenario", "test-empty-probes", "-base", bin, "-head", bin, "-runs", "1", "-warmup", "0"}, extra...)
		if code := dispatch(args, &out, &errb); code != 2 {
			t.Errorf("%v: exit %d, want 2 (%s)", extra, code, errb.String())
		}
		if !strings.Contains(errb.String(), "no metrics") || out.Len() != 0 {
			t.Errorf("%v: err %q out %q", extra, errb.String(), out.String())
		}
	}
}
