package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestRegressedNeedsRatioAndDelta(t *testing.T) {
	base := Metric{P50: 11}
	cases := []struct {
		head float64
		want bool
	}{
		{11, false},   // identical
		{13.5, false}, // ratio 1.23 < 1.25
		{14.5, true},  // ratio 1.32 and +3.5 ms
		{22, true},    // 2x
		{13.7, false}, // ratio 1.245 < 1.25
	}
	for _, c := range cases {
		if got := regressed(base, Metric{P50: c.head}, 1.25, 3); got != c.want {
			t.Errorf("head %v: got %v want %v", c.head, got, c.want)
		}
	}
	// Large ratio but tiny absolute delta (timer noise on a 1 ms probe) passes.
	if regressed(Metric{P50: 1}, Metric{P50: 2}, 1.25, 3) {
		t.Error("1 ms -> 2 ms must not fail with a 3 ms floor")
	}
}

func TestCompareReportsMissingMetric(t *testing.T) {
	base := []Metric{{Name: "a", P50: 10}, {Name: "b", P50: 10}}
	head := []Metric{{Name: "a", P50: 10}}
	got := compare(base, head, 1.25, 3)
	if len(got) != 1 || got[0] != "b" {
		t.Errorf("got %v", got)
	}
}

func TestABSelfTestInjectedSlowdownFails(t *testing.T) {
	bin := script(t, "nself", "exit 0")
	args := []string{"ab", "-base", bin, "-head", bin, "-runs", "6", "-warmup", "1", "-min-delta-ms", "1", "-json"}

	var out, errb bytes.Buffer
	var res ABResult
	code := dispatch(append(append([]string{}, args...), "-inject-slowdown", "30"), &out, &errb)
	if code != 1 {
		t.Fatalf("injected: exit %d (%s)", code, errb.String())
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "fail" || !res.Injected || len(res.Failed) != 3 || len(res.Base.Metrics) != 3 || len(res.Head.Metrics) != 3 {
		t.Errorf("got verdict %s injected %v failed %v", res.Verdict, res.Injected, res.Failed)
	}
}

func TestABUsageErrors(t *testing.T) {
	var out, errb bytes.Buffer
	if code := dispatch([]string{"ab"}, &out, &errb); code != 2 {
		t.Errorf("no flags: exit %d", code)
	}
	if code := dispatch([]string{"ab", "-base", "x", "-head", "y"}, &out, &errb); code != 2 {
		t.Errorf("missing binaries: exit %d", code)
	}
}
