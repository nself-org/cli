package vectorsearch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// harness builds Deps with in-memory fakes and records what happened.
type harness struct {
	colType  string
	tracked  bool
	present  bool
	colErr   error
	drift    []string
	trackErr error
	tracks   [][]string
	deps     Deps
	clock    time.Time
}

func newHarness() *harness {
	h := &harness{colType: "vector(3)", tracked: true, present: true, clock: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)}
	h.deps = Deps{
		Exec: func(context.Context, string, []string) (string, string, error) { return h.colType + "\n", "", nil },
		PG:   PG{Container: "demo_postgres", User: "postgres", DB: "nself"},
		TableTracked: func(string, string, string) (bool, bool, error) {
			return h.present, h.tracked, nil
		},
		ColumnVisible: func(string, string, string, string) error { return h.colErr },
		Drift:         func(context.Context, string) ([]string, error) { return h.drift, nil },
		Track: func(_ context.Context, schema, name, table, column string, roles []string) error {
			h.tracks = append(h.tracks, append([]string{schema, name, table, column}, roles...))
			return h.trackErr
		},
		Now: func() time.Time { return h.clock },
	}
	return h
}

func opts(dir string, track bool) Options {
	return Options{Spec: Spec{Schema: "public", Table: "docs", Column: "embedding"}, ProjectDir: dir, Track: track}
}

func TestRunWritesMigrationOnly(t *testing.T) {
	h, dir := newHarness(), t.TempDir()
	res, err := Run(t.Context(), h.deps, opts(dir, false))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "migrations", "20261010120000_vector_search_docs_embedding.sql")
	if res.Migration != want || !res.MigrationNew || res.Dim != 3 || res.Tracked || len(h.tracks) != 0 {
		t.Fatalf("result = %+v tracks=%v", res, h.tracks)
	}
	got, _ := os.ReadFile(want)
	golden, _ := os.ReadFile("testdata/golden/docs_embedding_cosine.sql")
	if string(got) != string(golden) {
		t.Errorf("migration differs from the golden SQL:\n%s", got)
	}
}

func TestRunRejectsBeforeWriting(t *testing.T) {
	for name, mut := range map[string]func(*harness){
		"text column":      func(h *harness) { h.colType = "text" },
		"untracked":        func(h *harness) { h.tracked = false },
		"no disk metadata": func(h *harness) { h.present = false },
		"hidden column":    func(h *harness) { h.colErr = errors.New("E514 stand-in") },
	} {
		t.Run(name, func(t *testing.T) {
			h, dir := newHarness(), t.TempDir()
			mut(h)
			_, err := Run(t.Context(), h.deps, opts(dir, true))
			if err == nil || !strings.Contains(err.Error(), "E513") && !strings.Contains(err.Error(), "E514") {
				t.Fatalf("err = %v, want E513 or E514", err)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Errorf("files written despite the error: %v", entries)
			}
			if len(h.tracks) != 0 {
				t.Errorf("tracked despite the error: %v", h.tracks)
			}
		})
	}
}

func TestRunReusesIdenticalMigration(t *testing.T) {
	h, dir := newHarness(), t.TempDir()
	first, err := Run(t.Context(), h.deps, opts(dir, false))
	if err != nil {
		t.Fatal(err)
	}
	h.clock = h.clock.Add(time.Hour)
	second, err := Run(t.Context(), h.deps, opts(dir, false))
	if err != nil || second.Migration != first.Migration || second.MigrationNew {
		t.Fatalf("second run = %+v, %v; want the first file reused", second, err)
	}
	// A different distance is new SQL: a new file.
	o := opts(dir, false)
	o.Spec.Distance = L2
	third, err := Run(t.Context(), h.deps, o)
	if err != nil || !third.MigrationNew || third.Migration == first.Migration {
		t.Fatalf("l2 run = %+v, %v; want a new file", third, err)
	}
}

func TestRunTrackMirrorsWhenClean(t *testing.T) {
	h := newHarness()
	dir, file := writeFixture(t, "yaml")
	res, err := Run(t.Context(), h.deps, opts(dir, true))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Tracked || res.DiskMetadata != "updated" || len(h.tracks) != 1 {
		t.Fatalf("result = %+v tracks=%v", res, h.tracks)
	}
	if got := strings.Join(h.tracks[0], ","); got != "public,docs_nearest_embedding,docs,embedding,user" {
		t.Errorf("Track args = %s", got)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "docs_nearest_embedding") {
		t.Errorf("function not mirrored:\n%s", data)
	}
}

func TestRunTrackSkipsMirrorOnDrift(t *testing.T) {
	h := newHarness()
	h.drift = []string{"sources"}
	dir, file := writeFixture(t, "yaml")
	before, _ := os.ReadFile(file)
	res, err := Run(t.Context(), h.deps, opts(dir, true))
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(file)
	if !res.Tracked || res.DiskMetadata != "skipped" || string(before) != string(after) {
		t.Fatalf("result = %+v; disk changed=%v", res, string(before) != string(after))
	}
	if !strings.Contains(res.Note, "differs from live Hasura") {
		t.Errorf("note = %q", res.Note)
	}
}

func TestRunTrackErrorsPropagate(t *testing.T) {
	h := newHarness()
	dir, _ := writeFixture(t, "yaml")
	h.trackErr = errors.New("E514 stand-in")
	if res, err := Run(t.Context(), h.deps, opts(dir, true)); err == nil || res.Tracked {
		t.Fatalf("track error was swallowed: %+v %v", res, err)
	}
	h2 := newHarness()
	h2.deps.Drift = func(context.Context, string) ([]string, error) { return nil, errors.New("hasura down") }
	if _, err := Run(t.Context(), h2.deps, opts(dir, true)); err == nil || len(h2.tracks) != 0 {
		t.Fatalf("drift failure must stop before tracking: %v tracks=%v", err, h2.tracks)
	}
}
