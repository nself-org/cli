package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/cli/tools/perfbench/scenarios"
)

func TestPercentileNearestRank(t *testing.T) {
	// 1..10: p50 = ceil(0.5*10)=5th = 5; p95 = ceil(9.5)=10th = 10.
	s := []float64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	if got := percentile(s, 50); got != 5 {
		t.Errorf("p50 = %v, want 5", got)
	}
	if got := percentile(s, 95); got != 10 {
		t.Errorf("p95 = %v, want 10", got)
	}
	// n=30: p95 = ceil(28.5) = 29th value.
	var big []float64
	for i := 1; i <= 30; i++ {
		big = append(big, float64(i))
	}
	if got := percentile(big, 95); got != 29 {
		t.Errorf("n=30 p95 = %v, want 29", got)
	}
	if got := percentile([]float64{7}, 95); got != 7 {
		t.Errorf("n=1 p95 = %v, want 7", got)
	}
	if got := percentile(nil, 50); got != 0 {
		t.Errorf("empty p50 = %v, want 0", got)
	}
	if s[0] != 10 {
		t.Error("percentile modified its input")
	}
}

func TestSummarizeRoundsToTenth(t *testing.T) {
	m := summarize("x", "ms", []float64{1.04, 1.06, 2.25, 9.949})
	if m.P50 != 1.1 || m.P95 != 9.9 || m.Max != 9.9 || m.N != 4 {
		t.Errorf("got %+v", m)
	}
}

func TestResultJSONKeyOrderIsDeterministic(t *testing.T) {
	r := Result{Schema: schemaV1, Scenario: "cold-start", SHA: "abc", GOOS: "linux", GOARCH: "amd64", Runs: 30,
		Metrics: []Metric{{Name: "cold_start.version", Unit: "ms", P50: 10.5, P95: 18.6, Max: 29.8, N: 30}}}
	out, err := marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "schema": "perfbench/v1",
  "scenario": "cold-start",
  "sha": "abc",
  "goos": "linux",
  "goarch": "amd64",
  "runs": 30,
  "injected": false,
  "metrics": [
    {
      "name": "cold_start.version",
      "unit": "ms",
      "p50": 10.5,
      "p95": 18.6,
      "max": 29.8,
      "n": 30
    }
  ]
}
`
	if string(out) != want {
		t.Errorf("JSON mismatch:\n%s\nwant:\n%s", out, want)
	}
	var back Result
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
}

func TestGroupSamplesOrderAndMixedUnits(t *testing.T) {
	ms, err := groupSamples([]scenarios.Sample{
		{Metric: "b", Unit: "ms", Value: 2}, {Metric: "a", Unit: "ms", Value: 1}, {Metric: "b", Unit: "ms", Value: 4},
	})
	if err != nil || len(ms) != 2 || ms[0].Name != "b" || ms[1].Name != "a" || ms[0].N != 2 {
		t.Fatalf("got %+v, %v", ms, err)
	}
	_, err = groupSamples([]scenarios.Sample{{Metric: "m", Unit: "ms", Value: 1}, {Metric: "m", Unit: "s", Value: 1}})
	if err == nil || !strings.Contains(err.Error(), "mixed units") {
		t.Errorf("want mixed-units error, got %v", err)
	}
}
