package commands

// bundle_appready_track_test.go — P7-PLUG-78: the TrackTables hook of bundle
// install app-ready is wired in v1.5 only and names the plugin it fails for.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
)

// trackProject makes a project dir the hooks resolve their config from.
func trackProject(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=trk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)
}

// stubTracker swaps bundleTrackTables and records every call.
func stubTracker(t *testing.T, fn func([]database.PluginTables) error) *[][]database.PluginTables {
	t.Helper()
	old := bundleTrackTables
	var calls [][]database.PluginTables
	bundleTrackTables = func(_ context.Context, _ *config.Config, p []database.PluginTables) (database.TrackResult, error) {
		calls = append(calls, p)
		if fn != nil {
			return database.TrackResult{}, fn(p)
		}
		return database.TrackResult{}, nil
	}
	t.Cleanup(func() { bundleTrackTables = old })
	return &calls
}

// TestBundleAppReadyTrackHookWired: v1.5 wires TrackTables and it hands the
// members' tables (entries as written) to the tracker; v1.4 wires nothing.
func TestBundleAppReadyTrackHookWired(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		h := newBundleAppReadyHooks(bundleInstallCmd, "task")
		if !compat.V15() {
			if h.TrackTables != nil {
				t.Fatal("v1.4 must not wire TrackTables")
			}
			return
		}
		if h.TrackTables == nil {
			t.Fatal("v1.5 must wire TrackTables")
		}
		trackProject(t)
		calls := stubTracker(t, nil)
		members := []appready.Member{
			{Name: "alpha", Tables: []string{"np_a_one", "ext.np_a_two"}},
			{Name: "empty"},
			{Name: "beta", Tables: []string{"np_b_one"}},
		}
		if err := h.TrackTables(context.Background(), members); err != nil {
			t.Fatal(err)
		}
		want := []database.PluginTables{{Plugin: "alpha", Tables: []string{"np_a_one", "ext.np_a_two"}}, {Plugin: "beta", Tables: []string{"np_b_one"}}}
		if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], want) {
			t.Fatalf("tracker calls = %+v, want one call with %+v", *calls, want)
		}
		// No member declares tables: no tracker call, no Hasura traffic.
		*calls = nil
		if err := h.TrackTables(context.Background(), []appready.Member{{Name: "empty"}}); err != nil || len(*calls) != 0 {
			t.Fatalf("no tables: err=%v calls=%d", err, len(*calls))
		}
	})
}

// TestBundleAppReadyTrackHookNamesPlugin: a rejection for one table fails for
// that plugin; a failure about no one plugin names all of them.
func TestBundleAppReadyTrackHookNamesPlugin(t *testing.T) {
	compattest.Set(t, true)
	trackProject(t)
	h := newBundleAppReadyHooks(bundleInstallCmd, "task")
	members := []appready.Member{{Name: "alpha", Tables: []string{"a"}}, {Name: "beta", Tables: []string{"b"}}}

	stubTracker(t, func([]database.PluginTables) error {
		return &database.TrackError{Plugin: "beta", Table: "public.b", Err: errors.New("plugin beta: table public.b does not exist in Postgres")}
	})
	var se *appready.StepError
	if err := h.TrackTables(context.Background(), members); !errors.As(err, &se) || se.Plugin != "beta" {
		t.Fatalf("err = %v, want a StepError for beta", err)
	}
	stubTracker(t, func([]database.PluginTables) error { return errors.New("connection refused") })
	err := h.TrackTables(context.Background(), members)
	if !errors.As(err, &se) || se.Plugin != "" || !strings.Contains(err.Error(), "alpha, beta") || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("err = %v, want a bundle-level StepError naming alpha, beta", err)
	}
}

// TestTrackQualificationMatchesAppready pins database.QualifyPluginTable to the
// rule appready.Missing applies ("x" is public.x), so tracking and verifying agree.
func TestTrackQualificationMatchesAppready(t *testing.T) {
	for _, tbl := range []string{"x", "np_a_t", "s.x", "audit.np_a_log"} {
		_, miss := appready.Missing([]appready.Member{{Name: "p", Tables: []string{tbl}}}, map[string]bool{})
		if len(miss) != 1 || miss[0] != database.QualifyPluginTable(tbl) {
			t.Errorf("%q: appready says %v, database says %q", tbl, miss, database.QualifyPluginTable(tbl))
		}
	}
}
