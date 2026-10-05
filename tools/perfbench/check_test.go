package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const budgetJSON = `{"_note":"n","schema":"perfbench-budget/v1","platform":"p","p95_ms":{"cold_start.version":150,"cold_start.help":150}}`

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// resultJSON renders a run result with the given metrics.
func resultJSON(t *testing.T, injected bool, ms ...Metric) string {
	t.Helper()
	r := newResult("cold-start", "", 30, ms)
	r.Injected = injected
	out, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func runCheck(t *testing.T, budget, result string, extra ...string) (int, string, string) {
	t.Helper()
	args := append([]string{"check", "-budget", writeFile(t, "b.json", budget), "-in", writeFile(t, "r.json", result)}, extra...)
	var out, errb bytes.Buffer
	code := dispatch(args, &out, &errb)
	return code, out.String(), errb.String()
}

func good(name string, p95 float64) Metric {
	return Metric{Name: name, Unit: "ms", P50: 10, P95: p95, Max: p95, N: 30}
}

func TestCheckPassesWithinBudget(t *testing.T) {
	code, out, errs := runCheck(t, budgetJSON, resultJSON(t, false, good("cold_start.version", 149.9), good("cold_start.help", 12)))
	if code != 0 || !strings.Contains(out, "cold_start.version p95 149.9 ms <= budget 150.0 ms") {
		t.Errorf("exit %d out %q err %q", code, out, errs)
	}
}

func TestCheckFailsOverBudgetNamingMetricValueBudget(t *testing.T) {
	code, _, errs := runCheck(t, budgetJSON, resultJSON(t, false, good("cold_start.version", 150.1), good("cold_start.help", 12)))
	if code != 1 || !strings.Contains(errs, "cold_start.version p95 150.1 ms > budget 150.0 ms") {
		t.Errorf("exit %d err %q", code, errs)
	}
	if strings.Contains(errs, "cold_start.help") {
		t.Errorf("within-budget metric reported: %q", errs)
	}
}

func TestCheckRefusesInjectedResult(t *testing.T) {
	code, _, errs := runCheck(t, budgetJSON, resultJSON(t, true, good("cold_start.version", 5), good("cold_start.help", 5)))
	if code != 2 || !strings.Contains(errs, "injected: true") {
		t.Errorf("exit %d err %q", code, errs)
	}
}

// Every way a result can fail to prove a metric is under its budget fails.
func TestCheckFailsClosed(t *testing.T) {
	cases := []struct {
		name, result, want string
		code               int
	}{
		{"metric missing", resultJSON(t, false, good("cold_start.version", 5)), "cold_start.help: budgeted but missing", 1},
		{"too few samples", resultJSON(t, false, Metric{Name: "cold_start.version", Unit: "ms", P95: 5, N: 19}, good("cold_start.help", 5)), "only 19 samples", 1},
		{"zero timing", resultJSON(t, false, good("cold_start.version", 0), good("cold_start.help", 5)), "not a usable timing", 1},
		{"wrong unit", resultJSON(t, false, Metric{Name: "cold_start.version", Unit: "s", P95: 1, N: 30}, good("cold_start.help", 5)), `unit "s"`, 1},
		{"unparseable", `{"schema":`, "result", 2},
		{"wrong schema", `{"schema":"x/v9","metrics":[]}`, `schema "x/v9"`, 2},
		{"no metrics", resultJSON(t, false), "missing from the result", 1},
	}
	for _, c := range cases {
		code, _, errs := runCheck(t, budgetJSON, c.result)
		if code != c.code || !strings.Contains(errs, c.want) {
			t.Errorf("%s: exit %d err %q", c.name, code, errs)
		}
	}
}

func TestCheckRejectsBadBudgets(t *testing.T) {
	res := resultJSON(t, false, good("cold_start.version", 5), good("cold_start.help", 5))
	cases := map[string]string{
		"empty map":      `{"schema":"perfbench-budget/v1","p95_ms":{}}`,
		"wrong schema":   `{"schema":"perfbench-budget/v2","p95_ms":{"a":1}}`,
		"zero budget":    `{"schema":"perfbench-budget/v1","p95_ms":{"cold_start.version":0}}`,
		"unknown key":    `{"schema":"perfbench-budget/v1","p95ms":{"a":1},"p95_ms":{"a":1}}`,
		"not json":       `p95: 150`,
		"missing schema": `{"p95_ms":{"cold_start.version":150}}`,
	}
	for name, b := range cases {
		if code, _, errs := runCheck(t, b, res); code != 2 {
			t.Errorf("%s: exit %d err %q", name, code, errs)
		}
	}
	var out, errb bytes.Buffer
	if code := dispatch([]string{"check", "-budget", "/nonexistent", "-in", "/nonexistent"}, &out, &errb); code != 2 {
		t.Errorf("missing files: exit %d", code)
	}
	if code := dispatch([]string{"check"}, &out, &errb); code != 2 {
		t.Errorf("no flags: exit %d", code)
	}
}

// An `ab` document is accepted as input (its run fields describe the head).
func TestCheckReadsAnABDocument(t *testing.T) {
	r := newResult("cold-start", "", 40, []Metric{good("cold_start.version", 9), good("cold_start.help", 9)})
	doc, err := json.Marshal(ABResult{Result: r, Verdict: "pass", Failed: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if code, _, errs := runCheck(t, budgetJSON, string(doc)); code != 0 {
		t.Errorf("exit %d %q", code, errs)
	}
}

// The committed budget file must load and cover the three cold-start probes.
func TestCommittedBudgetFile(t *testing.T) {
	b, err := loadBudget(filepath.Join("..", "..", ".github", "perf-budget.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"cold_start.version", "cold_start.help", "cold_start.status"} {
		if b.P95MS[n] != 150 {
			t.Errorf("%s: budget %v, want 150", n, b.P95MS[n])
		}
	}
}
