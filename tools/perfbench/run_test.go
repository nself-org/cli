package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

func TestColdStartRunsRealProcesses(t *testing.T) {
	bin := script(t, "nself", "exit 0")
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

func TestProbeExitCodeChangeIsAnError(t *testing.T) {
	// Alternates 0, 1, 0, 1 ... using a counter file outside HOME and cwd.
	counter := filepath.Join(t.TempDir(), "n")
	bin := script(t, "nself", fmt.Sprintf(`n=$(cat %[1]s 2>/dev/null || echo 0); echo $((n+1)) > %[1]s; exit $((n %% 2))`, counter))
	var out, errb bytes.Buffer
	code := dispatch([]string{"run", "-bin", bin, "-runs", "4", "-warmup", "0"}, &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "exit code changed") {
		t.Errorf("exit %d %q", code, errb.String())
	}
}

func TestProbeRunsInCleanEnvironment(t *testing.T) {
	// The probe must see telemetry opted out, a HOME that is not ours and an empty cwd.
	bin := script(t, "nself", `test "$NSELF_TELEMETRY_OPT_OUT" = 1 || exit 7
test "$HOME" != "`+os.Getenv("HOME")+`" || exit 8
test -z "$(ls -A .)" || exit 9
exit 0`)
	ms, exit, err := scenarios.RunOnce(context.Background(), bin, nil, []string{"version"}, 0)
	if err != nil || exit != 0 || ms <= 0 {
		t.Errorf("ms %v exit %d err %v", ms, exit, err)
	}
}
