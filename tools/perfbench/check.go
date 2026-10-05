package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
)

// budgetSchemaV1 is the value of the "schema" field of the budget file
// (.github/perf-budget.json).
const budgetSchemaV1 = "perfbench-budget/v1"

// defaultMinN is the fewest samples a metric may carry for `check` to trust
// its p95. With nearest-rank percentiles, p95 of fewer than 20 samples is the
// maximum, so a smaller run cannot prove a 95th percentile is under a budget.
const defaultMinN = 20

// Budget is the .github/perf-budget.json document. P95MS maps a metric name
// to its absolute p95 ceiling in milliseconds. Note is free text (the update
// rule lives in the wiki) and is ignored.
type Budget struct {
	Note     string             `json:"_note"`
	Schema   string             `json:"schema"`
	Platform string             `json:"platform"`
	P95MS    map[string]float64 `json:"p95_ms"`
}

// checkBudget compares res against b and returns one line per metric that
// breaks the budget, in metric-name order, plus one line per metric that
// passed. It fails closed: a budget metric absent from the result, a metric
// with fewer than minN samples, a unit other than ms, or a p95 that is not a
// positive finite number is a violation, never a pass.
func checkBudget(b Budget, res Result, minN int) (violations, passes []string) {
	byName := map[string]Metric{}
	for _, m := range res.Metrics {
		byName[m.Name] = m
	}
	names := make([]string, 0, len(b.P95MS))
	for n := range b.P95MS {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		budget := b.P95MS[n]
		m, ok := byName[n]
		switch {
		case !ok:
			violations = append(violations, fmt.Sprintf("%s: budgeted but missing from the result", n))
		case m.Unit != "ms":
			violations = append(violations, fmt.Sprintf("%s: unit %q, want ms", n, m.Unit))
		case m.N < minN:
			violations = append(violations, fmt.Sprintf("%s: only %d samples, need at least %d for a p95", n, m.N, minN))
		case math.IsNaN(m.P95) || math.IsInf(m.P95, 0) || m.P95 <= 0:
			violations = append(violations, fmt.Sprintf("%s: p95 %v ms is not a usable timing", n, m.P95))
		case m.P95 > budget:
			violations = append(violations, fmt.Sprintf("%s p95 %.1f ms > budget %.1f ms", n, m.P95, budget))
		default:
			passes = append(passes, fmt.Sprintf("%s p95 %.1f ms <= budget %.1f ms (n=%d)", n, m.P95, budget, m.N))
		}
	}
	return violations, passes
}

// loadBudget reads and validates the budget file. Unknown fields are errors
// so a misspelt key cannot silently disable a ceiling.
func loadBudget(path string) (Budget, error) {
	var b Budget
	raw, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return b, fmt.Errorf("budget %s: %w", path, err)
	}
	if b.Schema != budgetSchemaV1 {
		return b, fmt.Errorf("budget %s: schema %q, want %q", path, b.Schema, budgetSchemaV1)
	}
	if len(b.P95MS) == 0 {
		return b, fmt.Errorf("budget %s: p95_ms is empty; an empty budget would pass everything", path)
	}
	for n, v := range b.P95MS {
		if math.IsNaN(v) || v <= 0 {
			return b, fmt.Errorf("budget %s: %s has budget %v, want a positive number", path, n, v)
		}
	}
	return b, nil
}

// loadResult reads a perfbench/v1 document (a `run` result, or an `ab`
// document, whose run fields describe the head). A result produced with
// -inject-slowdown is refused: its timings are fake by construction.
func loadResult(path string) (Result, error) {
	var r Result
	raw, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return r, fmt.Errorf("result %s: %w", path, err)
	}
	if r.Schema != schemaV1 {
		return r, fmt.Errorf("result %s: schema %q, want %q", path, r.Schema, schemaV1)
	}
	if r.Injected {
		return r, fmt.Errorf("result %s: produced with an injected slowdown (injected: true); refusing to check it", path)
	}
	return r, nil
}

// checkCmd implements `perfbench check -budget <file> -in <result.json>`.
// Exit 0 when every budgeted metric is within its ceiling, 1 on a violation,
// 2 on a usage error or an unreadable, invalid or injected input.
func checkCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	budgetPath := fs.String("budget", "", "budget file (perfbench-budget/v1)")
	inPath := fs.String("in", "", "perfbench/v1 result file to check")
	minN := fs.Int("min-n", defaultMinN, "fewest samples a budgeted metric may have")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *budgetPath == "" || *inPath == "" || *minN < 1 {
		sayln(stderr, "check: -budget and -in are required and -min-n must be >= 1")
		return 2
	}
	b, err := loadBudget(*budgetPath)
	if err != nil {
		sayln(stderr, "check:", err)
		return 2
	}
	res, err := loadResult(*inPath)
	if err != nil {
		sayln(stderr, "check:", err)
		return 2
	}
	violations, passes := checkBudget(b, res, *minN)
	if len(passes) > 0 {
		say(stdout, "%s\n", strings.Join(passes, "\n"))
	}
	if len(violations) > 0 {
		say(stderr, "check: FAIL\n%s\n", strings.Join(violations, "\n"))
		return 1
	}
	return 0
}
