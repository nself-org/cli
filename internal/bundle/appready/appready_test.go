package appready

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOrderAndPrintPlan(t *testing.T) {
	want := []string{"licence", "fetch", "install", "ledger", "reconcile", "start", "readiness", "tracking", "health"}
	if !reflect.DeepEqual(Order, want) {
		t.Fatalf("Order = %v", Order)
	}
	var b bytes.Buffer
	PrintPlan(&b, "task", []string{"a", "b"})
	last := -1
	for _, s := range want {
		i := strings.Index(b.String(), " "+s+" ")
		if i <= last {
			t.Fatalf("%s out of order in\n%s", s, b.String())
		}
		last = i
	}
	if !strings.Contains(b.String(), "planned") {
		t.Error("dry-run lines must say planned")
	}
}

// TestRunnerStopsAtFirstFailure: after a failure later steps are skipped, not run.
func TestRunnerStopsAtFirstFailure(t *testing.T) {
	rn := NewRunner("task")
	ran := []string{}
	step := func(name string, err error) {
		_ = rn.Step(name, func() (string, error) { ran = append(ran, name); return "d", err })
	}
	step(StepLicence, nil)
	step(StepInstall, &StepError{Plugin: "jobs", Err: errors.New("boom\nsecond line")})
	step(StepLedger, nil)
	if !reflect.DeepEqual(ran, []string{"licence", "install"}) {
		t.Fatalf("ran = %v", ran)
	}
	rep := rn.Report()
	if rep.AppReady || rep.Failed != "install" || rep.FailedFor != "jobs" || len(rep.Steps) != 9 {
		t.Fatalf("report = %+v", rep)
	}
	got := map[string]StepResult{}
	for _, s := range rep.Steps {
		got[s.Name] = s
	}
	if got["install"].Result != ResultFailed || got["install"].Detail != "boom" || got["ledger"].Result != ResultSkipped || got["health"].Result != ResultSkipped {
		t.Errorf("steps = %+v", got)
	}
	var b bytes.Buffer
	Render(&b, rep)
	if !strings.Contains(b.String(), "NOT APP-READY: step install failed for plugin jobs") {
		t.Errorf("render:\n%s", b.String())
	}
	raw, _ := json.Marshal(rep)
	if !strings.Contains(string(raw), `"app_ready":false`) || !strings.Contains(string(raw), `"duration_ms"`) {
		t.Errorf("json = %s", raw)
	}
}

func TestRecordFailedMakesAFailure(t *testing.T) {
	rn := NewRunner("b")
	rn.Record(StepFetch, ResultFailed, time.Millisecond, "registry down")
	if f := rn.Failure(); f == nil || f.Step != StepFetch || rn.Report().AppReady {
		t.Fatalf("failure = %+v", f)
	}
}

// fixture helpers

func writeManifest(t *testing.T, dir, name string, extra map[string]any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "plugin", "manifestv2", "testdata", "v2", "full.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["name"], m["schema"] = name, "np_"+name
	for k, v := range extra {
		if v == nil {
			delete(m, k) // nil drops the key
		} else {
			m[k] = v
		}
	}
	out, _ := json.Marshal(m)
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, "plugin.json"), out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMembers(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "a", map[string]any{"tables": []string{"np_a_t", "np_a_u"}})
	writeManifest(t, dir, "b", map[string]any{"migrations": nil})
	if err := os.MkdirAll(filepath.Join(dir, "b", "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	ms, err := LoadMembers(dir, []string{"a", "b"})
	if err != nil || len(ms) != 2 {
		t.Fatalf("LoadMembers: %v %v", ms, err)
	}
	if !ms[0].BootMigrations || ms[0].Port != 3999 || ms[0].HealthEndpoint != "/health" || len(ms[0].Tables) != 2 || ms[0].HasMigrationsDir {
		t.Errorf("a = %+v", ms[0])
	}
	if ms[1].BootMigrations || !ms[1].HasMigrationsDir {
		t.Errorf("b = %+v", ms[1])
	}
	_, err = LoadMembers(dir, []string{"missing"})
	var se *StepError
	if !errors.As(err, &se) || se.Plugin != "missing" {
		t.Errorf("missing manifest error = %v", err)
	}
}

func TestTrackedTables(t *testing.T) {
	raw := `{"sources":[{"name":"default","tables":[
	  {"table":{"schema":"public","name":"np_a_t"}},
	  {"table":{"name":"np_a_noschema"}},
	  {"table":{"schema":"audit","name":"np_b_log"}}]},
	  {"name":"other","tables":[{"table":{"schema":"public","name":"np_c_t"}}]}],
	  "tables":[{"table":{"schema":"public","name":"np_legacy"}}]}`
	set, err := TrackedTables([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"public.np_a_t", "public.np_a_noschema", "audit.np_b_log", "public.np_c_t", "public.np_legacy"} {
		if !set[want] {
			t.Errorf("missing %s in %v", want, set)
		}
	}
	if set["np_b_log"] || set["public.np_b_log"] {
		t.Error("schema must not be dropped: audit.np_b_log is not public.np_b_log")
	}
	wrapped, _ := TrackedTables([]byte(`{"metadata":{"sources":[{"tables":[{"table":{"schema":"public","name":"w"}}]}]}}`))
	if !wrapped["public.w"] {
		t.Errorf("wrapped metadata = %v", wrapped)
	}
	if _, err := TrackedTables([]byte("<html>")); err == nil {
		t.Error("non-JSON must be an error")
	}
}

func TestMissingHandlesSchemas(t *testing.T) {
	tracked := map[string]bool{"public.np_a_t": true, "audit.np_a_log": true}
	ms := []Member{{Name: "a", Tables: []string{"np_a_t", "audit.np_a_log"}}, {Name: "b", Tables: []string{"np_b_z", "np_b_a"}}}
	name, miss := Missing(ms, tracked)
	if name != "b" || !reflect.DeepEqual(miss, []string{"public.np_b_a", "public.np_b_z"}) {
		t.Fatalf("Missing = %q %v", name, miss)
	}
	if name, _ := Missing(ms[:1], tracked); name != "" {
		t.Errorf("a is fully tracked, got %q", name)
	}
}

func health(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(s.Close)
	return s
}

func fastHooks(h Hooks) Hooks {
	h.Timeout, h.Interval = 150*time.Millisecond, 10*time.Millisecond
	return h
}

// TestRunStepOrderAndStop: steps run reconcile, start, readiness, tracking,
// health in order; the first failure skips the rest and names the plugin.
func TestRunStepOrderAndStop(t *testing.T) {
	dir := t.TempDir()
	ok := health(t, 200, `{"migrations":{"applied":1,"expected":1}}`)
	url := func(Member) string { return ok.URL }
	writeManifest(t, dir, "a", map[string]any{"tables": []string{"np_a_t"}})
	var order []string
	mark := func(n string) func(context.Context) error {
		return func(context.Context) error { order = append(order, n); return nil }
	}
	meta := `{"sources":[{"tables":[{"table":{"schema":"public","name":"np_a_t"}}]}]}`
	h := fastHooks(Hooks{Reconcile: mark("reconcile"), Start: mark("start"), HealthURL: url,
		ExportMetadata: func(context.Context) ([]byte, error) { order = append(order, "tracking"); return []byte(meta), nil },
		ServiceHealth:  mark("health")})
	rn := NewRunner("task")
	out := Run(context.Background(), rn, h, dir, []string{"a"}, true)
	if !out.Reconciled || !reflect.DeepEqual(order, []string{"reconcile", "start", "tracking", "health"}) {
		t.Fatalf("outcome %+v order %v", out, order)
	}
	if rep := rn.Report(); !rep.AppReady || rep.Failed != "" {
		t.Fatalf("report = %+v", rep)
	}

	// Missing table: tracking fails and health is skipped.
	order = nil
	h.ExportMetadata = func(context.Context) ([]byte, error) { return []byte(`{"sources":[{"tables":[]}]}`), nil }
	rn = NewRunner("task")
	Run(context.Background(), rn, h, dir, []string{"a"}, true)
	rep := rn.Report()
	if rep.Failed != "tracking" || rep.FailedFor != "a" || !strings.Contains(rn.Failure().Err.Error(), "public.np_a_t") {
		t.Fatalf("tracking failure report = %+v err=%v", rep, rn.Failure())
	}
	last := rep.Steps[len(rep.Steps)-1]
	if last.Name != "health" || last.Result != ResultSkipped {
		t.Errorf("health after a failure = %+v", last)
	}
}

func TestRunReconcileSkippedWhenUnchangedAndFailureStops(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "a", nil)
	var reconciles, starts int32
	h := fastHooks(Hooks{
		Reconcile: func(context.Context) error { atomic.AddInt32(&reconciles, 1); return nil },
		Start:     func(context.Context) error { atomic.AddInt32(&starts, 1); return errors.New("compose up failed") },
	})
	rn := NewRunner("b")
	out := Run(context.Background(), rn, h, dir, []string{"a"}, false)
	if out.Reconciled || reconciles != 0 || starts != 1 {
		t.Fatalf("unchanged: reconciled=%v reconciles=%d starts=%d", out.Reconciled, reconciles, starts)
	}
	if f := rn.Failure(); f == nil || f.Step != "start" || f.Plugin != "(bundle)" {
		t.Fatalf("failure = %+v", f)
	}
	// A failure before Run (ledger) means no hook runs and nothing is marked reconciled.
	rn = NewRunner("b")
	rn.Record(StepLedger, ResultFailed, 0, "disk full")
	if out := Run(context.Background(), rn, h, dir, []string{"a"}, true); out.Reconciled || reconciles != 0 {
		t.Errorf("hooks ran after a failed ledger step: %+v", out)
	}
}

func TestRunReadinessFailureNamesPlugin(t *testing.T) {
	dir := t.TempDir()
	bad := health(t, 200, `{"migrations":{"applied":0,"expected":2}}`)
	writeManifest(t, dir, "slow", nil)
	h := fastHooks(Hooks{Reconcile: func(context.Context) error { return nil }, Start: func(context.Context) error { return nil },
		HealthURL: func(Member) string { return bad.URL }, ExportMetadata: func(context.Context) ([]byte, error) { return nil, nil }})
	rn := NewRunner("b")
	Run(context.Background(), rn, h, dir, []string{"slow"}, true)
	f := rn.Failure()
	if f == nil || f.Step != "readiness" || f.Plugin != "slow" || !strings.Contains(f.Err.Error(), "0 of 2") {
		t.Fatalf("failure = %+v", f)
	}
}

// TestRunNoBootMigrationsNamesE120: a plugin with a migrations dir but no boot
// declaration is not waited on; the step detail names E120.
func TestRunNoBootMigrationsNamesE120(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "legacy", map[string]any{"migrations": nil})
	if err := os.MkdirAll(filepath.Join(dir, "legacy", "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	ms, _ := LoadMembers(dir, []string{"legacy"})
	detail, err := waitMembers(context.Background(), fastHooks(Hooks{}), ms)
	if err != nil || !strings.Contains(detail, "E120") || !strings.Contains(detail, "legacy") {
		t.Fatalf("detail=%q err=%v", detail, err)
	}
}

func TestRunNoProjectSkipsRest(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "a", nil)
	h := fastHooks(Hooks{Reconcile: func(context.Context) error { return fmt.Errorf("wrapped: %w", ErrNoProject) },
		Start: func(context.Context) error { t.Error("start ran"); return nil }})
	rn := NewRunner("b")
	out := Run(context.Background(), rn, h, dir, []string{"a"}, true)
	rep := rn.Report()
	if !out.Reconciled || rn.Failure() != nil || rep.AppReady || rep.Note == "" {
		t.Fatalf("outcome %+v report %+v", out, rep)
	}
	for _, s := range rep.Steps[4:] {
		if s.Result != ResultSkipped || s.Detail != "not in an nSelf project" {
			t.Errorf("step %+v", s)
		}
	}
}

// TestHealthStepWaitsForPlugin: /health must answer 200; a 503 that turns 200
// counts, a permanent 503 fails naming the plugin. A 200 without a migrations
// field (E119) is healthy.
func TestHealthStepWaitsForPlugin(t *testing.T) {
	var n int32
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&n, 1) < 3 {
			w.WriteHeader(503)
			return
		}
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer flaky.Close()
	h := fastHooks(Hooks{HealthURL: func(Member) string { return flaky.URL }})
	if _, err := checkHealth(context.Background(), h, []Member{{Name: "p", Port: 1}}); err != nil {
		t.Fatalf("flaky then healthy: %v", err)
	}
	down := health(t, 503, "")
	h.HealthURL = func(Member) string { return down.URL }
	_, err := checkHealth(context.Background(), h, []Member{{Name: "p", Port: 1}})
	var se *StepError
	if !errors.As(err, &se) || se.Plugin != "p" {
		t.Fatalf("permanent 503: err = %v", err)
	}
	// ServiceHealth failure comes first.
	h.ServiceHealth = func(context.Context) error { return errors.New("unhealthy: hasura") }
	if _, err := checkHealth(context.Background(), h, nil); err == nil || !strings.Contains(err.Error(), "hasura") {
		t.Errorf("ServiceHealth error = %v", err)
	}
}

func TestDefaultHealthURLAndTimeout(t *testing.T) {
	m := Member{Name: "p", Port: 4001, HealthEndpoint: "/healthz"}
	if got := (Hooks{}).healthURL(m); got != "http://127.0.0.1:4001/healthz" {
		t.Errorf("default URL = %s", got)
	}
	t.Setenv("NSELF_PLUGIN_READY_TIMEOUT", "7")
	if got := (Hooks{}).timeout(); got != 7*time.Second {
		t.Errorf("timeout from env = %s", got)
	}
	if got := (Hooks{Timeout: time.Second}).timeout(); got != time.Second {
		t.Errorf("explicit timeout = %s", got)
	}
}
