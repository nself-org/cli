package scenarios

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// Probe is one named measurement of a fresh process: the binary is started
// with Args and its wall time (start to exit) is the sample, in milliseconds.
type Probe struct {
	Metric string
	Args   []string
}

// Prober is implemented by scenarios made of fixed probes. `perfbench ab`
// needs it to interleave two binaries over the same probes.
type Prober interface {
	Probes() []Probe
}

// RunOnce runs bin with args in a clean environment and returns the wall time
// in milliseconds and the exit code. The probe gets a fresh empty HOME and an
// empty working directory, telemetry off, stdin at /dev/null and stdout and
// stderr discarded. Any exit code is returned, not treated as an error; only a
// failure to start the process is an error.
//
// slowdown > 1 (self-test only) makes the timed window F times the measured
// time by sleeping before the clock stops.
func RunOnce(ctx context.Context, bin string, extraEnv, args []string, slowdown float64) (float64, int, error) {
	home, err := os.MkdirTemp("", "perfbench-home-*")
	if err != nil {
		return 0, 0, err
	}
	defer os.RemoveAll(home)
	work, err := os.MkdirTemp("", "perfbench-cwd-*")
	if err != nil {
		return 0, 0, err
	}
	defer os.RemoveAll(work)

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = work
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"NSELF_TELEMETRY_OPT_OUT=1",
	}, extraEnv...)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return 0, 0, fmt.Errorf("start %s: %w", bin, err)
	}
	waitErr := cmd.Wait()
	elapsed := time.Since(start)
	if slowdown > 1 {
		time.Sleep(time.Duration(float64(elapsed) * (slowdown - 1)))
		elapsed = time.Since(start)
	}
	exit := 0
	if waitErr != nil {
		var ee *exec.ExitError
		if !errors.As(waitErr, &ee) {
			return 0, 0, fmt.Errorf("wait %s: %w", bin, waitErr)
		}
		exit = ee.ExitCode()
	}
	return float64(elapsed) / float64(time.Millisecond), exit, nil
}

// ExitTracker remembers the first exit code seen per key and reports any later
// run that differs: a probe whose exit code changes between runs is measuring
// something unstable and must fail the run.
type ExitTracker map[string]int

// Check records exit for key, or returns an error if it differs from the first.
func (t ExitTracker) Check(key string, exit int) error {
	first, seen := t[key]
	if !seen {
		t[key] = exit
		return nil
	}
	if first != exit {
		return fmt.Errorf("probe %s: exit code changed between runs (%d, then %d)", key, first, exit)
	}
	return nil
}

// RunProbes measures every probe cfg.Runs times after cfg.Warmup discarded
// runs and returns one ms sample per measured run, probe by probe.
func RunProbes(ctx context.Context, cfg Config, probes []Probe) ([]Sample, error) {
	exits := ExitTracker{}
	var out []Sample
	for _, p := range probes {
		for i := 0; i < cfg.Warmup+cfg.Runs; i++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			ms, exit, err := RunOnce(ctx, cfg.Bin, cfg.Env, p.Args, 0)
			if err != nil {
				return nil, err
			}
			if err := exits.Check(p.Metric, exit); err != nil {
				return nil, err
			}
			if i >= cfg.Warmup {
				out = append(out, Sample{Metric: p.Metric, Unit: "ms", Value: ms})
			}
		}
	}
	return out, nil
}
