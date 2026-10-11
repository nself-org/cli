package appready

// appready_fix2_test.go — P7-PLUG-19 fix round 2: tracking ignores table shapes
// and sources it does not understand (S3); the health step is healthy only on
// success or an answered 200 without a migrations field (S4).

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestTrackedTablesSkipsOtherShapesAndSources: a data-connector source exports
// "table" as an array, BigQuery as {dataset,name}; neither may fail the parse or
// count as public.<name>. The postgres source in the same document still counts.
func TestTrackedTablesSkipsOtherShapesAndSources(t *testing.T) {
	raw := `{"sources":[
	 {"name":"chinook","kind":"sqlite","tables":[{"table":["Album"]},{"table":"Artist"}]},
	 {"name":"bq","kind":"bigquery","tables":[{"table":{"dataset":"d","name":"events"}}]},
	 {"name":"default","kind":"postgres","tables":[
	   {"table":{"schema":"public","name":"np_a_t"}},
	   {"table":{"name":"np_a_noschema"}},
	   {"table":["Album"]},
	   {"table":{"dataset":"d","name":"leak"}}]},
	 {"name":"nokind","tables":[{"table":{"schema":"audit","name":"np_a_log"}}]},
	 {"name":"cit","kind":"citus","tables":[{"table":{"schema":"public","name":"np_c"}}]}]}`
	set, err := TrackedTables([]byte(raw))
	if err != nil {
		t.Fatalf("a non-postgres shape must not fail the parse: %v", err)
	}
	for _, want := range []string{"public.np_a_t", "public.np_a_noschema", "audit.np_a_log", "public.np_c"} {
		if !set[want] {
			t.Errorf("%s not tracked: %v", want, set)
		}
	}
	for _, bad := range []string{"public.events", "public.Album", "public.Artist", "public.leak", "d.events"} {
		if set[bad] {
			t.Errorf("%s must be ignored: %v", bad, set)
		}
	}
	if len(set) != 4 {
		t.Errorf("set = %v, want exactly the four postgres tables", set)
	}
}

// TestTrackedTablesErrorMessages: invalid JSON and a wrong top-level type get
// different messages.
func TestTrackedTablesErrorMessages(t *testing.T) {
	if _, err := TrackedTables([]byte("<html>")); err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Errorf("syntax error = %v, want 'not JSON'", err)
	}
	_, err := TrackedTables([]byte(`{"sources":"oops"}`))
	if err == nil || strings.Contains(err.Error(), "not JSON") || !strings.Contains(err.Error(), "unexpected shape") {
		t.Errorf("type error = %v, want 'unexpected shape' and not 'not JSON'", err)
	}
}

// TestHealthStepFailsOnBadURL: a malformed health URL is not "healthy" and is
// not retried until the timeout.
func TestHealthStepFailsOnBadURL(t *testing.T) {
	h := fastHooks(Hooks{HealthURL: func(Member) string { return "http://127.0.0.1:3105health" }})
	h.Timeout = 5 * time.Second // a retry loop would take this long
	start := time.Now()
	_, err := checkHealth(context.Background(), h, []Member{{Name: "p", Port: 1}})
	if err == nil || !strings.Contains(err.Error(), "bad health URL") {
		t.Fatalf("bad URL: err = %v, want a failure naming the URL", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("a bad URL must fail at once, took %s", time.Since(start))
	}
}

// TestHealthStepE119IsHealthy: 200 with a body lacking the migrations field is
// the only non-nil probe error that still proves health.
func TestHealthStepE119IsHealthy(t *testing.T) {
	srv := health(t, 200, `{"status":"ok"}`)
	h := fastHooks(Hooks{HealthURL: func(Member) string { return srv.URL }})
	if _, err := checkHealth(context.Background(), h, []Member{{Name: "p", Port: 1}}); err != nil {
		t.Errorf("E119 answer must count as healthy: %v", err)
	}
}

// TestLoadMembersNormalisesHealthEndpoint: "health" becomes "/health".
func TestLoadMembersNormalisesHealthEndpoint(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "a", map[string]any{"health_endpoint": "healthz"})
	ms, err := LoadMembers(dir, []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if ms[0].HealthEndpoint != "/healthz" {
		t.Errorf("HealthEndpoint = %q, want /healthz", ms[0].HealthEndpoint)
	}
	if got := DefaultHealthURL(ms[0]); got != "http://127.0.0.1:3999/healthz" {
		t.Errorf("URL = %s", got)
	}
}
