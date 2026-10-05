package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	bin := script(t, "nself", okScript)
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

// A head that crashes at startup looks "faster"; the verdict must fail instead.
func TestABFailsWhenHeadIsDead(t *testing.T) {
	base := script(t, "base", okScript)
	cases := []struct {
		name, body, want string
		extra            []string
	}{
		{"head exits 1", `exit 1`, "probe cold_start.version (head): exit code 1, want 0", nil},
		{"head killed", `kill -9 $$`, "(head): exit code -1", nil},
		{"head hangs", `exec sleep 30`, "(head): ", []string{"-timeout", "300ms"}},
	}
	for _, c := range cases {
		head := script(t, "head", c.body)
		var out, errb bytes.Buffer
		args := append([]string{"ab", "-base", base, "-head", head, "-runs", "3", "-warmup", "0", "-json"}, c.extra...)
		code := dispatch(args, &out, &errb)
		if code != 1 || !strings.Contains(errb.String(), c.want) {
			t.Errorf("%s: exit %d stderr %q", c.name, code, errb.String())
			continue
		}
		var res ABResult
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			t.Fatalf("%s: %v\n%s", c.name, err, out.String())
		}
		if res.Verdict != "fail" || len(res.Failed) != 1 || res.Failed[0] != "cold_start.version" || res.Error == "" {
			t.Errorf("%s: %+v", c.name, res)
		}
	}
}

func TestABFailsWhenBaseIsDead(t *testing.T) {
	head := script(t, "head", okScript)
	base := script(t, "base", `exit 2`)
	var out, errb bytes.Buffer
	code := dispatch([]string{"ab", "-base", base, "-head", head, "-runs", "2", "-warmup", "0"}, &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "(base): exit code 2") {
		t.Errorf("exit %d %q", code, errb.String())
	}
}

// Identical samples pass; 2x fails; 1.2x passes; 1.3x with a 2 ms delta passes;
// a failing probe is named in `failed`.
func TestCompareDecisions(t *testing.T) {
	mk := func(name string, p50 float64) []Metric { return []Metric{{Name: name, P50: p50}} }
	cases := []struct {
		name       string
		base, head float64
		fail       bool
	}{
		{"identical", 12, 12, false},
		{"2x", 12, 24, true},
		{"1.2x", 12, 14.4, false},
		{"1.3x with 2 ms delta", 6, 7.8, false},
		{"1.3x with 3.6 ms delta", 12, 15.6, true},
		{"faster", 12, 6, false},
	}
	for _, c := range cases {
		got := compare(mk("cold_start.help", c.base), mk("cold_start.help", c.head), 1.25, 3)
		if (len(got) == 1 && got[0] == "cold_start.help") != c.fail || len(got) > 1 {
			t.Errorf("%s: failed=%v", c.name, got)
		}
	}
}

// The measuring order alternates each round: base first on even rounds, head first on odd.
func TestABAlternatesOrder(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "order.log")
	body := `echo "$0" | sed 's|.*/||' >> ` + log + `; case "$1" in status) exit 1;; *) exit 0;; esac`
	base := script(t, "base", body)
	head := script(t, "head", body)
	var out, errb bytes.Buffer
	if code := dispatch([]string{"ab", "-base", base, "-head", head, "-runs", "3", "-warmup", "1", "-scenario", "cold-start"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d %s", code, errb.String())
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Fields(string(raw))
	if len(lines) != 3*4*2 { // 3 probes x (1 warm-up + 3 runs) x 2 binaries
		t.Fatalf("got %d runs: %v", len(lines), lines)
	}
	for i := 0; i < 4; i++ { // first probe, four rounds
		first, second := lines[2*i], lines[2*i+1]
		wantFirst, wantSecond := "base", "head"
		if i%2 == 1 {
			wantFirst, wantSecond = "head", "base"
		}
		if first != wantFirst || second != wantSecond {
			t.Errorf("round %d: ran %s then %s, want %s then %s", i, first, second, wantFirst, wantSecond)
		}
	}
}
