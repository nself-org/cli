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
	shape    string // psql answer to the function-shape query: "t", "f" or ""
	drift    []string
	trackErr error
	tracks   [][]string
	deps     Deps
	clock    time.Time
}

func newHarness() *harness {
	h := &harness{colType: "vector(3)", tracked: true, present: true, shape: "t", clock: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)}
	h.deps = Deps{
		Exec: func(_ context.Context, _ string, cmd []string) (string, string, error) {
			if strings.Contains(strings.Join(cmd, " "), "pg_proc") {
				return h.shape + "\n", "", nil
			}
			return h.colType + "\n", "", nil
		},
		PG: PG{Container: "demo_postgres", User: "postgres", DB: "nself"},
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

func TestRunTrackRefusesWrongFunctionShape(t *testing.T) {
	for name, tc := range map[string]struct{ shape, want string }{
		"missing":   {"", "does not exist"},
		"not shape": {"f", "not the generated inlinable shape"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness()
			h.shape = tc.shape
			dir, _ := writeFixture(t, "yaml")
			_, err := Run(t.Context(), h.deps, opts(dir, true))
			if err == nil || !strings.Contains(err.Error(), "E513") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want E513 %q", err, tc.want)
			}
			if len(h.tracks) != 0 {
				t.Errorf("tracked despite the shape check: %v", h.tracks)
			}
		})
	}
	h := newHarness()
	h.deps.Exec = func(_ context.Context, _ string, cmd []string) (string, string, error) {
		if strings.Contains(strings.Join(cmd, " "), "pg_proc") {
			return "", "boom", errors.New("exec failed")
		}
		return "vector(3)\n", "", nil
	}
	dir, _ := writeFixture(t, "yaml")
	if _, err := Run(t.Context(), h.deps, opts(dir, true)); err == nil || strings.Contains(err.Error(), "E513") {
		t.Errorf("an exec failure must be a plain error, got %v", err)
	}
}

func TestShapeQuery(t *testing.T) {
	q, err := shapeQuery(docsSpec(Cosine))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"prosecdef", "proconfig IS NULL", "proretset", "provolatile = 's'", "c.reltype",
		"NOT p.proisstrict", `regexp_replace(btrim(p.prosrc, E' \t\r\n;'), '\s+', ' ', 'g')`,
		`= 'SELECT * FROM "public"."docs" ORDER BY "embedding" <=> $1'`,
		`to_regprocedure('"public"."docs_nearest_embedding"(vector)')`, `to_regclass('"public"."docs"')`} {
		if !strings.Contains(q, want) {
			t.Errorf("shape query lacks %q:\n%s", want, q)
		}
	}
	// D-0369: the compared body is exactly the one Render writes, per distance.
	for _, d := range []Distance{Cosine, L2, IP} {
		sp := docsSpec(d)
		sqlText, err := Render(sp)
		if err != nil {
			t.Fatal(err)
		}
		body := functionBody(`"public"`, `"docs"`, `"embedding"`, sp.Operator())
		if !strings.Contains(sqlText, "AS $$\n  "+body+"\n$$;") {
			t.Errorf("%s: rendered SQL does not hold the body %q:\n%s", d, body, sqlText)
		}
		q, err := shapeQuery(sp)
		if err != nil || !strings.Contains(q, "= '"+body+"'") {
			t.Errorf("%s: shape query does not compare the body %q: %v\n%s", d, body, err, q)
		}
	}
	bad := docsSpec(Cosine)
	bad.Table = "x'; DROP"
	if _, err := shapeQuery(bad); err == nil {
		t.Error("shapeQuery accepted an unvalidated table")
	}
	for _, f := range []func(*Spec){func(s *Spec) { s.Schema = "a b" }, func(s *Spec) { s.Column = "a b" }} {
		sp := docsSpec(Cosine)
		f(&sp)
		if _, err := shapeQuery(sp); err == nil {
			t.Error("shapeQuery accepted a bad identifier")
		}
	}
}

func TestWriteMigrationNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	s := docsSpec(Cosine)
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(dir, "20261010120000_vector_search_docs_embedding.sql")
	if err := os.WriteFile(path, []byte("-- someone else's SQL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, written, err := writeMigration(dir, s, "SELECT 1;\n", now)
	if err == nil || written || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v written=%v, want an already-exists error", err, written)
	}
	if got, _ := os.ReadFile(path); string(got) != "-- someone else's SQL\n" {
		t.Errorf("existing file was changed: %q", got)
	}
}

func TestRunMigrationDirErrors(t *testing.T) {
	h := newHarness()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "migrations")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil { // a file where the dir should be
		t.Fatal(err)
	}
	if _, err := Run(t.Context(), h.deps, opts(dir, false)); err == nil {
		t.Error("an unwritable migration dir must fail")
	}
}

func TestRunValidationAndLookupErrors(t *testing.T) {
	h := newHarness()
	o := opts(t.TempDir(), false)
	o.Spec.Table = "bad table"
	if _, err := Run(t.Context(), h.deps, o); err == nil {
		t.Error("invalid spec accepted")
	}
	h.deps.TableTracked = func(string, string, string) (bool, bool, error) { return false, false, errors.New("disk unreadable") }
	if _, err := Run(t.Context(), h.deps, opts(t.TempDir(), false)); err == nil || !strings.Contains(err.Error(), "disk unreadable") {
		t.Errorf("tracked-check error lost: %v", err)
	}
}

func TestRunTrackMirrorOutcomes(t *testing.T) {
	// unsupported layout: tracked live, disk skipped with a note
	h := newHarness()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "hasura", "metadata", "databases"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := Run(t.Context(), h.deps, opts(dir, true))
	if err != nil || !res.Tracked || res.DiskMetadata != "skipped" || !strings.Contains(res.Note, "left unchanged") {
		t.Fatalf("unsupported layout: %+v %v", res, err)
	}
	// the entry is already on disk: unchanged
	h2 := newHarness()
	dir2, _ := writeFixture(t, "json")
	if _, _, err := MirrorFunction(dir2, "public", "docs_nearest_embedding", []string{"user"}); err != nil {
		t.Fatal(err)
	}
	res, err = Run(t.Context(), h2.deps, opts(dir2, true))
	if err != nil || res.DiskMetadata != "unchanged" {
		t.Fatalf("already mirrored: %+v %v", res, err)
	}
	// a mirror failure that is not a layout problem is an error
	h3 := newHarness()
	dir3, file := writeFixture(t, "json")
	if err := os.WriteFile(file, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(t.Context(), h3.deps, opts(dir3, true)); err == nil {
		t.Error("a corrupt metadata.json must fail the mirror")
	}
}
