package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/cli/tools/perfbench/scenarios"
)

// schemaV1 is the value of the "schema" field of every result document.
const schemaV1 = "perfbench/v1"

// Metric is one summarized measurement. Field order is the JSON key order
// (contract:cli.perfbench v1).
type Metric struct {
	Name string  `json:"name"`
	Unit string  `json:"unit"`
	P50  float64 `json:"p50"`
	P95  float64 `json:"p95"`
	Max  float64 `json:"max"`
	N    int     `json:"n"`
}

// Result is the perfbench/v1 document. Field order is the JSON key order.
type Result struct {
	Schema   string   `json:"schema"`
	Scenario string   `json:"scenario"`
	SHA      string   `json:"sha"`
	GOOS     string   `json:"goos"`
	GOARCH   string   `json:"goarch"`
	Runs     int      `json:"runs"`
	Injected bool     `json:"injected"`
	Metrics  []Metric `json:"metrics"`
}

// newResult fills the environment fields of a result. sha identifies what was
// measured (see binarySHA); it is empty when unknown.
func newResult(scenario, sha string, runs int, metrics []Metric) Result {
	return Result{
		Schema:   schemaV1,
		Scenario: scenario,
		SHA:      sha,
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
		Runs:     runs,
		Metrics:  metrics,
	}
}

// marshal renders v as 2-space-indented JSON with a trailing newline.
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// treeSHA is the git HEAD of the checkout dir, with "-dirty" appended when
// tracked files are modified, or "" when dir is not a git checkout. It is used
// only for a binary perfbench built itself from that dir. A binary handed in
// with -bin gets no sha unless the caller states one with -sha: neither the
// harness checkout nor the Go toolchain's build stamp (which names the wrong
// repository inside a linked worktree) says what was measured.
func treeSHA(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	sha := strings.TrimSpace(string(out))
	st, err := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain", "--untracked-files=no").Output()
	if err == nil && len(strings.TrimSpace(string(st))) > 0 {
		sha += "-dirty"
	}
	return sha
}

// groupSamples folds samples into one metric per name, in first-appearance
// order. It fails when samples of one metric carry different units.
func groupSamples(samples []scenarios.Sample) ([]Metric, error) {
	var order []string
	units := map[string]string{}
	values := map[string][]float64{}
	for _, s := range samples {
		if u, seen := units[s.Metric]; !seen {
			order = append(order, s.Metric)
			units[s.Metric] = s.Unit
		} else if u != s.Unit {
			return nil, fmt.Errorf("metric %s: mixed units %q and %q", s.Metric, u, s.Unit)
		}
		values[s.Metric] = append(values[s.Metric], s.Value)
	}
	metrics := make([]Metric, 0, len(order))
	for _, name := range order {
		metrics = append(metrics, summarize(name, units[name], values[name]))
	}
	return metrics, nil
}

// formatMetrics renders metrics as aligned text lines.
func formatMetrics(ms []Metric) string {
	var b strings.Builder
	for _, m := range ms {
		fmt.Fprintf(&b, "%-22s p50 %8.1f %s  p95 %8.1f %s  max %8.1f %s  n=%d\n",
			m.Name, m.P50, m.Unit, m.P95, m.Unit, m.Max, m.Unit, m.N)
	}
	return b.String()
}
