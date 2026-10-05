package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/tools/perfbench/scenarios"
)

// fakeScenario proves the registration API (P7-CANON-21 adds scenarios this way).
type fakeScenario struct{ unitB string }

func (fakeScenario) Name() string { return "test-fake" }
func (f fakeScenario) Run(_ context.Context, cfg scenarios.Config) ([]scenarios.Sample, error) {
	var out []scenarios.Sample
	for i := 1; i <= cfg.Runs; i++ {
		out = append(out, scenarios.Sample{Metric: "fake.a", Unit: "ms", Value: float64(i)})
	}
	if f.unitB != "" {
		out = append(out, scenarios.Sample{Metric: "fake.a", Unit: f.unitB, Value: 1})
	}
	return out, nil
}

func init() { scenarios.Register(fakeScenario{}) }

func TestRegisteredScenarioRunsThroughRun(t *testing.T) {
	var out, errb bytes.Buffer
	code := dispatch([]string{"run", "-scenario", "test-fake", "-bin", "unused", "-runs", "10", "-json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	var res Result
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Schema != "perfbench/v1" || res.Scenario != "test-fake" || res.Runs != 10 || len(res.Metrics) != 1 ||
		res.Metrics[0].Name != "fake.a" || res.Metrics[0].P50 != 5 || res.Metrics[0].P95 != 10 || res.Metrics[0].N != 10 {
		t.Errorf("got %+v", res)
	}
}

func TestFlagFirstImpliesRun(t *testing.T) {
	var out, errb bytes.Buffer
	code := dispatch([]string{"--scenario", "test-fake", "--bin", "unused", "--runs", "3", "--json"}, &out, &errb)
	if code != 0 || !strings.Contains(out.String(), `"scenario": "test-fake"`) {
		t.Errorf("exit %d out %s err %s", code, out.String(), errb.String())
	}
}

func TestUnknownScenarioAndSubcommand(t *testing.T) {
	var out, errb bytes.Buffer
	if code := dispatch([]string{"run", "-scenario", "nope", "-bin", "x"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "cold-start") {
		t.Errorf("unknown scenario: exit %d %q", code, errb.String())
	}
	if code := dispatch([]string{"frobnicate"}, &out, &errb); code != 2 {
		t.Errorf("unknown subcommand: exit %d", code)
	}
}

func TestMixedUnitsFailTheRun(t *testing.T) {
	scenarios.Register(mixedScenario{})
	var out, errb bytes.Buffer
	code := dispatch([]string{"run", "-scenario", "test-mixed", "-bin", "x", "-runs", "2"}, &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "mixed units") {
		t.Errorf("exit %d %q", code, errb.String())
	}
}

type mixedScenario struct{ fakeScenario }

func (mixedScenario) Name() string { return "test-mixed" }
func (mixedScenario) Run(ctx context.Context, cfg scenarios.Config) ([]scenarios.Sample, error) {
	return fakeScenario{unitB: "s"}.Run(ctx, cfg)
}

// script writes an executable shell script and returns its path.
func script(t *testing.T, name, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts")
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// okScript behaves like nself for the cold-start probes: status exits 1, the rest 0.
const okScript = `case "$1" in status) exit 1;; *) exit 0;; esac`

func TestColdStartRunsRealProcesses(t *testing.T) {
	bin := script(t, "nself", okScript)
	var out, errb bytes.Buffer
	code := dispatch([]string{"run", "-bin", bin, "-runs", "4", "-warmup", "1", "-json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	var res Result
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, m := range res.Metrics {
		names = append(names, m.Name)
		if m.N != 4 || m.Unit != "ms" || m.P50 <= 0 {
			t.Errorf("metric %+v", m)
		}
	}
	if got := strings.Join(names, ","); got != "cold_start.version,cold_start.help,cold_start.status" {
		t.Errorf("metrics %s", got)
	}
}

// A binary that dies, exits with the wrong code or hangs must fail `run`, never
// produce a (fast) number.
func TestRunFailsOnDeadBinary(t *testing.T) {
	cases := []struct {
		name, body, want string
		extra            []string
	}{
		{"exits 1 immediately", `exit 1`, "probe cold_start.version: exit code 1, want 0", nil},
		{"killed by signal", `kill -9 $$`, "exit code -1, want 0", nil},
		{"status exits 0", `exit 0`, "probe cold_start.status: exit code 0, want 1", nil},
		{"hangs", `exec sleep 30`, "timed out after", []string{"-timeout", "300ms"}},
	}
	for _, c := range cases {
		bin := script(t, "nself", c.body)
		var out, errb bytes.Buffer
		args := append([]string{"run", "-bin", bin, "-runs", "2", "-warmup", "0", "-json"}, c.extra...)
		start := time.Now()
		code := dispatch(args, &out, &errb)
		if code != 1 || !strings.Contains(errb.String(), c.want) || out.Len() != 0 {
			t.Errorf("%s: exit %d stderr %q stdout %q, want exit 1 containing %q and no result", c.name, code, errb.String(), out.String(), c.want)
		}
		if time.Since(start) > 10*time.Second {
			t.Errorf("%s: took %s, the timeout did not fire", c.name, time.Since(start))
		}
	}
}

func TestWarmupRunsAreCheckedToo(t *testing.T) {
	bin := script(t, "nself", `exit 3`)
	var out, errb bytes.Buffer
	if code := dispatch([]string{"run", "-bin", bin, "-runs", "1", "-warmup", "2"}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "exit code 3") {
		t.Errorf("exit %d %q", code, errb.String())
	}
}

func TestResultSHAIsExplicitNeverTheHarnessCheckout(t *testing.T) {
	bin := script(t, "nself", okScript)
	var out, errb bytes.Buffer
	if code := dispatch([]string{"run", "-bin", bin, "-runs", "1", "-warmup", "0", "-json"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d %s", code, errb.String())
	}
	if !strings.Contains(out.String(), `"sha": ""`) {
		t.Errorf("sha must be empty for a -bin without -sha:\n%s", out.String())
	}
	out.Reset()
	if code := dispatch([]string{"run", "-bin", bin, "-sha", "ecaf2219", "-runs", "1", "-warmup", "0", "-json"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d %s", code, errb.String())
	}
	if !strings.Contains(out.String(), `"sha": "ecaf2219"`) {
		t.Errorf("-sha not recorded:\n%s", out.String())
	}
	out.Reset()
	if code := dispatch([]string{"ab", "-base", bin, "-head", bin, "-head-sha", "abc", "-runs", "1", "-warmup", "0",
		// This test checks the recorded sha, not the verdict. One unwarmed sample per side is
		// pure process-spawn noise (macos-15-intel flipped it to "fail"), so make the
		// regression thresholds unreachable.
		"-ratio", "1000000", "-min-delta-ms", "1000000", "-json"}, &out, &errb); code != 0 {
		t.Fatalf("ab exit %d %s", code, errb.String())
	}
	if !strings.Contains(out.String(), `"sha": "abc"`) {
		t.Errorf("-head-sha not recorded:\n%s", out.String())
	}
}

func TestTreeSHA(t *testing.T) {
	plain := t.TempDir()
	// TMPDIR may sit inside some other checkout; stop git's upward search at the temp dir.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(plain))
	if got := treeSHA(plain); got != "" {
		t.Errorf("non-checkout sha = %q, want empty", got)
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v %s", err, out)
		}
	}
	clean := treeSHA(dir)
	if len(clean) != 40 && len(clean) != 64 {
		t.Fatalf("clean sha = %q", clean)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "f"}} {
		_ = exec.Command("git", append([]string{"-C", dir}, args...)...).Run()
	}
	if got := treeSHA(dir); got != clean+"-dirty" {
		t.Errorf("dirty sha = %q, want %q", got, clean+"-dirty")
	}
}

func TestProbeRunsInCleanEnvironment(t *testing.T) {
	// The probe must see telemetry opted out, a HOME that is not ours and an empty cwd.
	bin := script(t, "nself", `test "$NSELF_TELEMETRY_OPT_OUT" = 1 || exit 7
test "$HOME" != "`+os.Getenv("HOME")+`" || exit 8
test -z "$(ls -A .)" || exit 9
exit 0`)
	ms, exit, err := scenarios.RunOnce(context.Background(), bin, []string{"version"}, scenarios.Opts{})
	if err != nil || exit != 0 || ms <= 0 {
		t.Errorf("ms %v exit %d err %v", ms, exit, err)
	}
}
