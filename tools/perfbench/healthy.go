package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// healthySteps are the golden-path steps whose durations add up to
// time-to-healthy: 3 (init) through 6 (health wait), per EPIC G8.
var healthySteps = []string{"3", "4", "5", "6"}

// goldenReport is the part of scripts/golden-path.sh's report that healthy
// reads: steps keyed "1".."13", each {status, duration, note}.
type goldenReport struct {
	Steps map[string]struct {
		Status   string  `json:"status"`
		Duration float64 `json:"duration"`
	} `json:"steps"`
}

// timeToHealthy sums the durations (seconds) of steps 3-6. It errors when any
// of them is missing or has a status other than pass or warn.
func timeToHealthy(data []byte) (float64, error) {
	var rep goldenReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return 0, fmt.Errorf("parse report: %w", err)
	}
	var bad []string
	sum := 0.0
	for _, id := range healthySteps {
		st, ok := rep.Steps[id]
		switch {
		case !ok:
			bad = append(bad, fmt.Sprintf("step %s missing", id))
		case st.Status != "pass" && st.Status != "warn":
			bad = append(bad, fmt.Sprintf("step %s status %q", id, st.Status))
		default:
			sum += st.Duration
		}
	}
	if len(bad) > 0 {
		return 0, fmt.Errorf("time_to_healthy not measurable: %s", strings.Join(bad, ", "))
	}
	return sum, nil
}

// healthyCmd implements `perfbench healthy -report <file> [-json]`.
func healthyCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("healthy", flag.ContinueOnError)
	fs.SetOutput(stderr)
	report := fs.String("report", "", "golden-path report JSON (scripts/golden-path.sh)")
	asJSON := fs.Bool("json", false, "print a perfbench/v1 document")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *report == "" {
		sayln(stderr, "healthy: -report is required")
		return 2
	}
	data, err := os.ReadFile(*report)
	if err != nil {
		sayln(stderr, "healthy:", err)
		return 2
	}
	sum, err := timeToHealthy(data)
	if err != nil {
		sayln(stderr, "healthy:", err)
		return 1
	}
	m := Metric{Name: "time_to_healthy", Unit: "s", P50: round1(sum), P95: round1(sum), Max: round1(sum), N: 1}
	if !*asJSON {
		say(stdout, "%s", formatMetrics([]Metric{m}))
		return 0
	}
	out, err := marshal(newResult("time-to-healthy", 1, []Metric{m}))
	if err != nil {
		sayln(stderr, "healthy:", err)
		return 2
	}
	put(stdout, out)
	return 0
}
