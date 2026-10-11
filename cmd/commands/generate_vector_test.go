package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database/vectorsearch"
)

// vectorFakes replaces the command's seams for one test.
type vectorFakes struct {
	colType string
	tracks  [][]string
}

func useVectorFakes(t *testing.T, f *vectorFakes) {
	t.Helper()
	old := vectorDepsFor
	t.Cleanup(func() { vectorDepsFor = old })
	vectorDepsFor = func(*config.Config) vectorsearch.Deps {
		return vectorsearch.Deps{
			Exec:          func(context.Context, string, []string) (string, string, error) { return f.colType + "\n", "", nil },
			PG:            vectorsearch.PG{Container: "demo_postgres", User: "postgres", DB: "nself"},
			TableTracked:  func(string, string, string) (bool, bool, error) { return true, true, nil },
			ColumnVisible: func(string, string, string, string) error { return nil },
			Drift:         func(context.Context, string) ([]string, error) { return []string{"sources"}, nil },
			Track: func(_ context.Context, schema, name, table, column string, roles []string) error {
				f.tracks = append(f.tracks, append([]string{schema, name, table, column}, roles...))
				return nil
			},
			Now: func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) },
		}
	}
}

func TestGenerateVectorCommand(t *testing.T) {
	t.Run("is a child of generate with its flags", func(t *testing.T) {
		found, _, err := generateCmd.Find([]string{"vector-search"})
		if err != nil || found != generateVectorSearchCmd {
			t.Fatalf("generate vector-search is not registered under generate: %v", err)
		}
		for _, f := range []string{"distance", "roles", "track", "json", "migration-dir"} {
			if generateVectorSearchCmd.Flags().Lookup(f) == nil {
				t.Errorf("missing flag --%s", f)
			}
		}
		if _, ok := jsonDataTypes["db generate vector-search"]; !ok {
			t.Error("db generate vector-search has no registered envelope type")
		}
	})

	t.Run("writes the migration and reports the next step", func(t *testing.T) {
		f := &vectorFakes{colType: "vector(3)"}
		useVectorFakes(t, f)
		dir := t.TempDir()
		mustWriteMeta(t, dir)
		out, _, err := runVectorIn(t, dir, []string{"public.docs.embedding"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		files, _ := filepath.Glob(filepath.Join(dir, "migrations", "*_vector_search_docs_embedding.sql"))
		if len(files) != 1 || len(f.tracks) != 0 {
			t.Fatalf("files=%v tracks=%v out=%s", files, f.tracks, out)
		}
		got, _ := os.ReadFile(files[0])
		for _, want := range []string{`CREATE OR REPLACE FUNCTION "public"."docs_nearest_embedding"(query vector(3))`, `ORDER BY "embedding" <=> $1`} {
			if !strings.Contains(string(got), want) {
				t.Errorf("migration lacks %q:\n%s", want, got)
			}
		}
		if !strings.Contains(out, "--track") {
			t.Errorf("output does not point at --track: %s", out)
		}
	})

	t.Run("--track tracks for the named roles and --json is one envelope", func(t *testing.T) {
		f := &vectorFakes{colType: "vector(3)"}
		useVectorFakes(t, f)
		dir := t.TempDir()
		mustWriteMeta(t, dir)
		doc := vectorCaptureStdout(t, func() {
			if _, _, err := runVectorIn(t, dir, []string{"public.docs.embedding"},
				map[string]string{"track": "true", "roles": "user,staff", "distance": "l2", "json": "true"}); err != nil {
				t.Fatal(err)
			}
		})
		if len(f.tracks) != 1 || strings.Join(f.tracks[0], ",") != "public,docs_nearest_embedding,docs,embedding,user,staff" {
			t.Fatalf("tracks = %v", f.tracks)
		}
		var env struct {
			Command string `json:"command"`
			Data    struct {
				Function     string `json:"function"`
				Tracked      bool   `json:"tracked"`
				DiskMetadata string `json:"disk_metadata"`
				Distance     string `json:"distance"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(doc), &env); err != nil {
			t.Fatalf("stdout is not one JSON document: %v\n%s", err, doc)
		}
		if env.Command != "db generate vector-search" || env.Data.Function != "public.docs_nearest_embedding" ||
			!env.Data.Tracked || env.Data.DiskMetadata != "skipped" || env.Data.Distance != "l2" {
			t.Errorf("envelope = %+v", env)
		}
	})

	t.Run("rejects bad input before touching anything", func(t *testing.T) {
		f := &vectorFakes{colType: "text"}
		useVectorFakes(t, f)
		dir := t.TempDir()
		mustWriteMeta(t, dir)
		for name, tc := range map[string]struct {
			args  []string
			flags map[string]string
			want  string
		}{
			"malformed target": {[]string{"docs"}, nil, "<schema>.<table>.<column>"},
			"bad distance":     {[]string{"public.docs.embedding"}, map[string]string{"distance": "dot"}, "invalid --distance"},
			"bad role":         {[]string{"public.docs.embedding"}, map[string]string{"roles": "Admin;x"}, "invalid role"},
			"text column":      {[]string{"public.docs.title"}, nil, "E513"},
		} {
			_, _, err := runVectorIn(t, dir, tc.args, tc.flags)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s: err = %v, want %q", name, err, tc.want)
			}
		}
		if files, _ := filepath.Glob(filepath.Join(dir, "migrations", "*")); len(files) != 0 {
			t.Errorf("files written: %v", files)
		}
	})
}

// mustWriteMeta is a placeholder project marker; the fakes decide tracking.
func mustWriteMeta(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "hasura", "metadata"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// runVectorIn is runVector inside an existing project directory.
func runVectorIn(t *testing.T, dir string, args []string, flags map[string]string) (string, string, error) {
	t.Helper()
	t.Chdir(dir)
	t.Setenv("PROJECT_NAME", "demo")
	cmd := generateVectorSearchCmd
	cmd.SetContext(t.Context())
	for _, name := range []string{"distance", "roles", "track", "migration-dir", "json"} {
		fl := cmd.Flags().Lookup(name)
		_ = fl.Value.Set(fl.DefValue)
		if sv, ok := fl.Value.(interface{ Replace([]string) error }); ok {
			_ = sv.Replace(nil)
		}
		fl.Changed = false
	}
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	err := runGenerateVectorSearch(cmd, args)
	return buf.String(), dir, err
}

// vectorCaptureStdout runs fn with os.Stdout redirected and returns what it wrote.
func vectorCaptureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stdout = old
	_ = w.Close()
	return <-done
}
