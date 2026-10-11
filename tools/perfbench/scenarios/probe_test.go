package scenarios

// Purpose: prove RunOnce, RunChecked and RunProbes against a stub binary (exit
// codes, signal death, timeout, start failure, warm-up, clean environment) and
// the HTTP readiness loop against an in-memory transport (D-0234).
// Constraints: the stub is a local shell script; no network, no docker.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// stubScript records "<args>|<HOME>|<PWD>|<telemetry>" per run and acts on $1.
const stubScript = `#!/bin/sh
echo "$*|$HOME|$(pwd)|$NSELF_TELEMETRY_OPT_OUT" >> "$STUB_OUT"
case "$1" in
  status) exit 1 ;;
  fail) exit 3 ;;
  hang) exec sleep 5 ;;
  nap) exec sleep 0.05 ;;
  die) kill -KILL $$ ;;
esac
exit 0
`

// newStub writes the stub binary and returns its path and its record file.
func newStub(t *testing.T) (bin, out string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub binary is a shell script")
	}
	dir := t.TempDir()
	bin, out = filepath.Join(dir, "nself"), filepath.Join(dir, "seen.txt")
	if err := os.WriteFile(bin, []byte(stubScript), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, out
}

// records returns the stub's recorded runs, one field slice per run.
func records(t *testing.T, out string) [][]string {
	t.Helper()
	b, err := os.ReadFile(out)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var rs [][]string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		rs = append(rs, strings.Split(l, "|"))
	}
	return rs
}

func TestProbeFailureError(t *testing.T) {
	cases := []struct {
		pf   ProbeFailure
		want string
	}{
		{ProbeFailure{Metric: "cold_start.version", Reason: "exit code 2, want 0"}, "probe cold_start.version: exit code 2, want 0"},
		{ProbeFailure{Metric: "cold_start.help", Side: "head", Reason: "timed out"}, "probe cold_start.help (head): timed out"},
	}
	for _, c := range cases {
		if got := c.pf.Error(); got != c.want {
			t.Errorf("Error() = %q, want %q", got, c.want)
		}
	}
}

func TestRunChecked(t *testing.T) {
	bin, out := newStub(t)
	env := []string{"STUB_OUT=" + out}
	cases := []struct {
		name    string
		bin     string
		probe   Probe
		timeout time.Duration
		reason  string // non-empty: a *ProbeFailure whose Reason contains this
		plain   bool   // a plain error, not a *ProbeFailure
	}{
		{name: "exit 0 as expected", bin: bin, probe: Probe{Metric: "m.ok", Args: []string{"version"}}},
		{name: "exit 1 as expected", bin: bin, probe: Probe{Metric: "m.status", Args: []string{"status"}, Exit: 1}},
		{name: "unexpected exit", bin: bin, probe: Probe{Metric: "m.fail", Args: []string{"fail"}}, reason: "exit code 3, want 0"},
		{name: "expected failure succeeds", bin: bin, probe: Probe{Metric: "m.status", Args: []string{"version"}, Exit: 1}, reason: "exit code 0, want 1"},
		{name: "signal death", bin: bin, probe: Probe{Metric: "m.die", Args: []string{"die"}}, reason: "exit code -1, want 0"},
		{name: "timeout", bin: bin, probe: Probe{Metric: "m.hang", Args: []string{"hang"}}, timeout: 100 * time.Millisecond, reason: "timed out"},
		{name: "start failure", bin: filepath.Join(t.TempDir(), "missing"), probe: Probe{Metric: "m.start", Args: []string{"version"}}, plain: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ms, err := RunChecked(context.Background(), c.bin, "base", c.probe, Opts{Env: env, Timeout: c.timeout})
			var pf *ProbeFailure
			switch {
			case c.plain:
				if err == nil || errors.As(err, &pf) {
					t.Fatalf("err = %v, want a plain error", err)
				}
			case c.reason != "":
				if !errors.As(err, &pf) || pf.Metric != c.probe.Metric || pf.Side != "base" || !strings.Contains(pf.Reason, c.reason) {
					t.Fatalf("err = %#v, want ProbeFailure{%s, base, ...%q...}", err, c.probe.Metric, c.reason)
				}
			default:
				if err != nil || ms <= 0 {
					t.Fatalf("ms=%v err=%v", ms, err)
				}
			}
		})
	}
}

// TestRunOnceEnvironment proves each run gets a fresh HOME and working
// directory (both removed afterwards), telemetry off and the extra Env.
func TestRunOnceEnvironment(t *testing.T) {
	bin, out := newStub(t)
	for i := 0; i < 2; i++ {
		if _, exit, err := RunOnce(context.Background(), bin, []string{"version", "x"}, Opts{Env: []string{"STUB_OUT=" + out}}); err != nil || exit != 0 {
			t.Fatalf("exit=%d err=%v", exit, err)
		}
	}
	rs := records(t, out)
	if len(rs) != 2 {
		t.Fatalf("%d runs recorded, want 2", len(rs))
	}
	realHome, _ := os.UserHomeDir()
	for _, r := range rs {
		args, home, cwd, telemetry := r[0], r[1], r[2], r[3]
		if args != "version x" || telemetry != "1" || home == realHome || home == cwd {
			t.Errorf("run saw args=%q HOME=%q cwd=%q telemetry=%q", args, home, cwd, telemetry)
		}
		for _, d := range []string{home, cwd} {
			if _, err := os.Stat(d); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s left behind (err=%v)", d, err)
			}
		}
	}
	if rs[0][1] == rs[1][1] {
		t.Errorf("both runs shared HOME %s", rs[0][1])
	}
}

func TestRunOnceSlowdown(t *testing.T) {
	bin, out := newStub(t)
	ms, _, err := RunOnce(context.Background(), bin, []string{"nap"}, Opts{Env: []string{"STUB_OUT=" + out}, Slowdown: 3})
	if err != nil {
		t.Fatal(err)
	}
	if ms < 140 { // >= 3 x the 50 ms nap
		t.Fatalf("slowed run took %.1f ms, want >= 140", ms)
	}
}

func TestRunProbes(t *testing.T) {
	bin, out := newStub(t)
	cfg := Config{Bin: bin, Runs: 2, Warmup: 1, Env: []string{"STUB_OUT=" + out}}
	cases := []struct {
		name     string
		run      func(context.Context, Config) ([]Sample, error)
		args     []string // recorded args, in order
		samples  []string // sample metrics, in order
		wantFail string   // non-empty: a *ProbeFailure for this metric
	}{
		{
			name:    "cold-start",
			run:     coldStart{}.Run,
			args:    []string{"version", "version", "version", "--help", "--help", "--help", "status", "status", "status"},
			samples: []string{"cold_start.version", "cold_start.version", "cold_start.help", "cold_start.help", "cold_start.status", "cold_start.status"},
		},
		{
			name:    "json-overhead",
			run:     jsonOverhead{}.Run,
			args:    []string{"version --json", "version --json", "version --json", "status --json", "status --json", "status --json"},
			samples: []string{"json_overhead.version", "json_overhead.version", "json_overhead.status", "json_overhead.status"},
		},
		{
			name: "warm-up failure stops at once",
			run: func(ctx context.Context, cfg Config) ([]Sample, error) {
				return RunProbes(ctx, cfg, []Probe{{Metric: "m.fail", Args: []string{"fail"}}, {Metric: "m.ok", Args: []string{"version"}}})
			},
			args:     []string{"fail"},
			wantFail: "m.fail",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_ = os.Remove(out)
			samples, err := c.run(context.Background(), cfg)
			var got []string
			for _, r := range records(t, out) {
				got = append(got, r[0])
			}
			if fmt.Sprint(got) != fmt.Sprint(c.args) {
				t.Errorf("stub ran %q, want %q", got, c.args)
			}
			if c.wantFail != "" {
				var pf *ProbeFailure
				if !errors.As(err, &pf) || pf.Metric != c.wantFail || pf.Side != "" || samples != nil {
					t.Fatalf("samples=%v err=%v, want a ProbeFailure for %s", samples, err, c.wantFail)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var metrics []string
			for _, s := range samples {
				metrics = append(metrics, s.Metric)
				if s.Unit != "ms" || s.Value <= 0 {
					t.Errorf("sample %+v", s)
				}
			}
			if fmt.Sprint(metrics) != fmt.Sprint(c.samples) {
				t.Errorf("samples %q, want %q", metrics, c.samples)
			}
		})
	}
}

func TestRunProbesCancelled(t *testing.T) {
	bin, out := newStub(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	samples, err := coldStart{}.Run(ctx, Config{Bin: bin, Runs: 1, Env: []string{"STUB_OUT=" + out}})
	if !errors.Is(err, context.Canceled) || samples != nil {
		t.Fatalf("samples=%v err=%v, want context.Canceled", samples, err)
	}
	if rs := records(t, out); len(rs) != 0 {
		t.Fatalf("stub ran %d times after cancel", len(rs))
	}
}

// roundTrip is an in-memory http.RoundTripper.
type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func reply(code int) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}
}

func TestWaitHTTPReady(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		answers []func() (*http.Response, error) // last one repeats
		wantErr error                            // nil: ready; else errors.Is target
		badURL  bool
		timeout time.Duration
	}{
		{name: "ready after refusal and 503", base: "http://127.0.0.1:1", answers: []func() (*http.Response, error){
			func() (*http.Response, error) { return nil, errors.New("connection refused") },
			func() (*http.Response, error) { return reply(http.StatusServiceUnavailable), nil },
			func() (*http.Response, error) { return reply(http.StatusOK), nil },
		}, timeout: 2 * time.Second}, // returns early; the slack absorbs -race stalls
		{name: "never ready", base: "http://127.0.0.1:1", answers: []func() (*http.Response, error){
			func() (*http.Response, error) { return reply(http.StatusUnauthorized), nil },
		}, wantErr: context.DeadlineExceeded, timeout: 150 * time.Millisecond},
		{name: "bad base URL", base: "http://bad\x7fhost", badURL: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/openapi.json" || r.Header.Get("Authorization") != "Bearer "+benchHTTPToken {
					t.Errorf("request %s %s auth=%q", r.Method, r.URL, r.Header.Get("Authorization"))
				}
				a := c.answers[min(calls, len(c.answers)-1)]
				calls++
				return a()
			})}
			err := waitHTTPReady(context.Background(), client, c.base, c.timeout)
			switch {
			case c.badURL:
				if err == nil || calls != 0 {
					t.Fatalf("err=%v calls=%d, want a URL error before any request", err, calls)
				}
			case c.wantErr != nil:
				if !errors.Is(err, c.wantErr) || !strings.Contains(err.Error(), "readiness") {
					t.Fatalf("err = %v, want readiness %v", err, c.wantErr)
				}
			default:
				if err != nil || calls != len(c.answers) {
					t.Fatalf("err=%v calls=%d, want ready on call %d", err, calls, len(c.answers))
				}
			}
		})
	}
}
