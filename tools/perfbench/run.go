package main

import (
	"context"
	"flag"
	"io"
	"strings"
	"time"

	"github.com/nself-org/cli/tools/perfbench/scenarios"
)

// runFlags are shared by `run` and, in part, `ab`.
type runFlags struct {
	scenario string
	bin      string
	src      string
	sha      string
	runs     int
	warmup   int
	asJSON   bool
	timeout  time.Duration
}

// runCmd implements `perfbench run [-scenario n] [-bin p] [-runs 30] [-warmup 3] [-json]`.
func runCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var f runFlags
	fs.StringVar(&f.scenario, "scenario", "cold-start", "scenario name ("+strings.Join(scenarios.Names(), ", ")+")")
	fs.StringVar(&f.bin, "bin", "", "nself binary to measure (default: build ./cmd/nself)")
	fs.StringVar(&f.src, "src", ".", "module directory to build when -bin is empty")
	fs.StringVar(&f.sha, "sha", "", "revision of the -bin binary, recorded in the result (default: empty)")
	fs.IntVar(&f.runs, "runs", 30, "measured runs per probe")
	fs.IntVar(&f.warmup, "warmup", 3, "discarded warm-up runs per probe")
	fs.BoolVar(&f.asJSON, "json", false, "print a perfbench/v1 document")
	fs.DurationVar(&f.timeout, "timeout", scenarios.DefaultTimeout, "kill and fail one probe run that takes longer")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if f.runs < 1 || f.warmup < 0 {
		sayln(stderr, "run: -runs must be >= 1 and -warmup >= 0")
		return 2
	}
	sc, ok := scenarios.Get(f.scenario)
	if !ok {
		say(stderr, "run: unknown scenario %q (have: %s)\n", f.scenario, strings.Join(scenarios.Names(), ", "))
		return 2
	}
	ctx := context.Background()
	bin, sha := f.bin, f.sha
	if bin == "" {
		built, cleanup, err := buildNself(ctx, f.src)
		if err != nil {
			sayln(stderr, "run:", err)
			return 2
		}
		defer cleanup()
		bin, sha = built, treeSHA(f.src)
	}
	samples, err := sc.Run(ctx, scenarios.Config{Bin: bin, Runs: f.runs, Warmup: f.warmup, Timeout: f.timeout})
	if err != nil {
		sayln(stderr, "run:", err)
		return 1
	}
	metrics, err := groupSamples(samples)
	if err != nil {
		sayln(stderr, "run:", err)
		return 1
	}
	if !f.asJSON {
		say(stdout, "%s", formatMetrics(metrics))
		return 0
	}
	out, err := marshal(newResult(f.scenario, sha, f.runs, metrics))
	if err != nil {
		sayln(stderr, "run:", err)
		return 2
	}
	put(stdout, out)
	return 0
}
