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

// DefaultTimeout bounds one probe run when Config.Timeout / Opts.Timeout is 0.
const DefaultTimeout = 30 * time.Second

// Probe is one named measurement of a fresh process: the binary is started
// with Args and its wall time (start to exit) is the sample, in milliseconds.
// Exit is the exit code the probe must produce (0 for most commands, 1 for a
// command that is expected to fail, such as `status` in an empty directory).
// Any other code, including death by signal, fails the run: a binary that
// crashes at startup is not fast.
type Probe struct {
	Metric string
	Args   []string
	Exit   int
}

// Prober is implemented by scenarios made of fixed probes. `perfbench ab`
// needs it to interleave two binaries over the same probes.
type Prober interface {
	Probes() []Probe
}

// Opts tunes one process run.
type Opts struct {
	Env      []string      // extra KEY=VALUE entries appended to the probe environment
	Slowdown float64       // self-test only: > 1 makes the timed window Slowdown x the measured time
	Timeout  time.Duration // kill the process after this long; 0 means DefaultTimeout
}

// ErrTimeout is returned (wrapped) by RunOnce when the process was killed for
// running past Opts.Timeout.
var ErrTimeout = errors.New("timed out")

// ProbeFailure is a measured run that must fail the scenario or the A/B
// verdict: an unexpected exit code, death by signal, or a timeout.
type ProbeFailure struct {
	Metric string
	Side   string // "base" or "head" for `ab`, empty for `run`
	Reason string
}

func (e *ProbeFailure) Error() string {
	side := ""
	if e.Side != "" {
		side = " (" + e.Side + ")"
	}
	return fmt.Sprintf("probe %s%s: %s", e.Metric, side, e.Reason)
}

// RunOnce runs bin with args in a clean environment and returns the wall time
// in milliseconds and the exit code (-1 when a signal killed the process). The
// probe gets a fresh empty HOME and an empty working directory, telemetry off,
// stdin at /dev/null and stdout and stderr discarded. Only a failure to start
// or a timeout is an error; judging the exit code is RunChecked's job.
func RunOnce(ctx context.Context, bin string, args []string, o Opts) (float64, int, error) {
	home, err := os.MkdirTemp("", "perfbench-home-*")
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = os.RemoveAll(home) }()
	work, err := os.MkdirTemp("", "perfbench-cwd-*")
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = os.RemoveAll(work) }()

	timeout := o.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = work
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"NSELF_TELEMETRY_OPT_OUT=1",
	}, o.Env...)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second // a killed process's children must not hold Wait open

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return 0, 0, fmt.Errorf("start %s: %w", bin, err)
	}
	waitErr := cmd.Wait()
	elapsed := time.Since(start)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return 0, 0, fmt.Errorf("%s %v: %w after %s", bin, args, ErrTimeout, timeout)
	}
	if o.Slowdown > 1 {
		time.Sleep(time.Duration(float64(elapsed) * (o.Slowdown - 1)))
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

// RunChecked runs probe p once and returns its wall time in ms. A timeout, a
// signal death or an exit code other than p.Exit is a *ProbeFailure naming the
// probe and side. side is "" outside `ab`.
func RunChecked(ctx context.Context, bin, side string, p Probe, o Opts) (float64, error) {
	ms, exit, err := RunOnce(ctx, bin, p.Args, o)
	if errors.Is(err, ErrTimeout) {
		return 0, &ProbeFailure{Metric: p.Metric, Side: side, Reason: err.Error()}
	}
	if err != nil {
		return 0, err
	}
	if exit != p.Exit {
		return 0, &ProbeFailure{Metric: p.Metric, Side: side,
			Reason: fmt.Sprintf("exit code %d, want %d (a negative code means a signal killed the process)", exit, p.Exit)}
	}
	return ms, nil
}

// RunProbes measures every probe cfg.Runs times after cfg.Warmup discarded
// runs and returns one ms sample per measured run, probe by probe. Warm-up
// runs are checked too: a binary that is dead on its first run fails at once.
func RunProbes(ctx context.Context, cfg Config, probes []Probe) ([]Sample, error) {
	var out []Sample
	for _, p := range probes {
		for i := 0; i < cfg.Warmup+cfg.Runs; i++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			ms, err := RunChecked(ctx, cfg.Bin, "", p, Opts{Env: cfg.Env, Timeout: cfg.Timeout})
			if err != nil {
				return nil, err
			}
			if i >= cfg.Warmup {
				out = append(out, Sample{Metric: p.Metric, Unit: "ms", Value: ms})
			}
		}
	}
	return out, nil
}
