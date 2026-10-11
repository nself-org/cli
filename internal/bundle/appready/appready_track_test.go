package appready

// Tests for the TrackTables hook (P7-PLUG-78): the tracking step tracks, then
// runs the existing verification read; a hook error fails the step naming the plugin.

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const trackedMeta = `{"sources":[{"tables":[{"table":{"schema":"public","name":"np_a_t"}}]}]}`

func trackHooks(t *testing.T, order *[]string) Hooks {
	t.Helper()
	ok := health(t, 200, `{"migrations":{"applied":1,"expected":1}}`)
	mark := func(n string) func(context.Context) error {
		return func(context.Context) error { *order = append(*order, n); return nil }
	}
	return fastHooks(Hooks{Reconcile: mark("reconcile"), Start: mark("start"), ServiceHealth: mark("health"),
		HealthURL: func(Member) string { return ok.URL }})
}

// TestAppReadyTrackingTracksThenVerifies: TrackTables runs before the export
// read, between readiness and health, and the verification sees what it tracked.
func TestAppReadyTrackingTracksThenVerifies(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "a", map[string]any{"tables": []string{"np_a_t"}})
	var order []string
	tracked := false
	h := trackHooks(t, &order)
	h.TrackTables = func(_ context.Context, ms []Member) error {
		order = append(order, "track")
		if len(ms) != 1 || ms[0].Name != "a" || !reflect.DeepEqual(ms[0].Tables, []string{"np_a_t"}) {
			t.Errorf("members = %+v", ms)
		}
		tracked = true
		return nil
	}
	h.ExportMetadata = func(context.Context) ([]byte, error) {
		order = append(order, "export")
		if !tracked {
			return []byte(`{"sources":[{"tables":[]}]}`), nil
		}
		return []byte(trackedMeta), nil
	}
	rn := NewRunner("task")
	Run(context.Background(), rn, h, dir, []string{"a"}, true)
	if want := []string{"reconcile", "start", "track", "export", "health"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	if rep := rn.Report(); !rep.AppReady || rep.Failed != "" {
		t.Fatalf("report = %+v", rep)
	}
}

// TestAppReadyTrackingHookFailureNamesPlugin: a TrackTables error fails the
// tracking step for that plugin, skips health, and never reaches the export read.
func TestAppReadyTrackingHookFailureNamesPlugin(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "a", map[string]any{"tables": []string{"np_a_t"}})
	var order []string
	h := trackHooks(t, &order)
	h.TrackTables = func(context.Context, []Member) error {
		return &StepError{Plugin: "a", Err: errors.New("plugin a: table public.np_a_t does not exist in Postgres")}
	}
	h.ExportMetadata = func(context.Context) ([]byte, error) { order = append(order, "export"); return nil, nil }
	rn := NewRunner("task")
	Run(context.Background(), rn, h, dir, []string{"a"}, true)
	f := rn.Failure()
	if f == nil || f.Step != StepTracking || f.Plugin != "a" || !strings.Contains(f.Err.Error(), "public.np_a_t") {
		t.Fatalf("failure = %+v", f)
	}
	for _, o := range order {
		if o == "export" || o == "health" {
			t.Fatalf("%s ran after the tracking failure: %v", o, order)
		}
	}
}

// TestAppReadyTrackingSkipsHookWithoutTables: a bundle whose members declare no
// tables never calls TrackTables (no Hasura traffic at all).
func TestAppReadyTrackingSkipsHookWithoutTables(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "a", map[string]any{"tables": nil})
	var order []string
	h := trackHooks(t, &order)
	h.TrackTables = func(context.Context, []Member) error { t.Error("TrackTables called with no tables"); return nil }
	h.ExportMetadata = func(context.Context) ([]byte, error) { t.Error("export read with no tables"); return nil, nil }
	rn := NewRunner("task")
	Run(context.Background(), rn, h, dir, []string{"a"}, true)
	if rep := rn.Report(); !rep.AppReady {
		t.Fatalf("report = %+v", rep)
	}
}
