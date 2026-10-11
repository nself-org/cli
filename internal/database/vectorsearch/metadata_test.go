package vectorsearch

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docsSources is the sources list a live export holds for a project that
// tracks public.docs with a user select filter (the shape HasuraExportToYAML
// and replace_metadata files use).
func docsSources() []any {
	return []any{map[string]any{
		"name": "default", "kind": "postgres",
		"configuration": map[string]any{"connection_info": map[string]any{
			"database_url": map[string]any{"from_env": "HASURA_GRAPHQL_DATABASE_URL"}, "use_prepared_statements": true}},
		"tables": []any{map[string]any{
			"table": map[string]any{"name": "docs", "schema": "public"},
			"select_permissions": []any{map[string]any{"role": "user", "permission": map[string]any{
				"columns": []any{"id", "owner"}, "filter": map[string]any{"owner": map[string]any{"_eq": "X-Hasura-User-Id"}}}}},
		}},
	}}
}

// writeFixture writes the before file of a layout and returns the project dir.
func writeFixture(t *testing.T, layout string) (dir, file string) {
	t.Helper()
	dir = t.TempDir()
	var data []byte
	var err error
	switch layout {
	case "yaml":
		file = filepath.Join(dir, "hasura", "metadata", "sources.yaml")
		data, err = marshalYAML(docsSources())
	case "json":
		file = filepath.Join(dir, "hasura", "metadata.json")
		data, err = marshalJSON(map[string]any{"version": 3, "sources": docsSources()})
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, file
}

// TestMirrorFunctionGolden mirrors public.docs_nearest_embedding for role user
// into both on-disk layouts and compares the result byte for byte with the
// golden files; the before files are pinned too so a format change is visible.
func TestMirrorFunctionGolden(t *testing.T) {
	for _, tc := range []struct{ layout, before, after string }{
		{"yaml", "yaml/sources.before.yaml", "yaml/sources.after.yaml"},
		{"json", "json/metadata.before.json", "json/metadata.after.json"},
	} {
		t.Run(tc.layout, func(t *testing.T) {
			dir, file := writeFixture(t, tc.layout)
			before, _ := os.ReadFile(file)
			assertGolden(t, tc.before, before)

			got, changed, err := MirrorFunction(dir, "public", "docs_nearest_embedding", []string{"user"})
			if err != nil || !changed || got != file {
				t.Fatalf("MirrorFunction = %q, %v, %v", got, changed, err)
			}
			after, _ := os.ReadFile(file)
			assertGolden(t, tc.after, after)

			// A second run is a no-op and leaves the bytes alone.
			_, changed, err = MirrorFunction(dir, "public", "docs_nearest_embedding", []string{"user"})
			again, _ := os.ReadFile(file)
			if err != nil || changed || string(again) != string(after) {
				t.Fatalf("second run changed=%v err=%v, bytes equal=%v", changed, err, string(again) == string(after))
			}
		})
	}
}

func TestMirrorFunctionAddsRoleToExistingEntry(t *testing.T) {
	dir, file := writeFixture(t, "yaml")
	if _, _, err := MirrorFunction(dir, "public", "docs_nearest_embedding", []string{"user"}); err != nil {
		t.Fatal(err)
	}
	_, changed, err := MirrorFunction(dir, "public", "docs_nearest_embedding", []string{"user", "staff"})
	if err != nil || !changed {
		t.Fatalf("adding a role: changed=%v err=%v", changed, err)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "role: staff") || strings.Count(string(data), "role: user") != 2 { // select permission + function permission
		t.Errorf("unexpected metadata after adding a role:\n%s", data)
	}
}

func TestMirrorFunctionUnsupportedLayouts(t *testing.T) {
	// No on-disk metadata at all: nothing is created.
	empty := t.TempDir()
	if _, _, err := MirrorFunction(empty, "public", "f", []string{"user"}); !errors.Is(err, ErrUnsupportedLayout) {
		t.Errorf("no metadata: err = %v, want ErrUnsupportedLayout", err)
	}
	if _, err := os.Stat(filepath.Join(empty, "hasura")); err == nil {
		t.Error("MirrorFunction created hasura/ in a project without metadata")
	}
	// A Hasura CLI layout (no sources.yaml) is left alone.
	cli := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cli, "hasura", "metadata", "databases"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := MirrorFunction(cli, "public", "f", []string{"user"}); !errors.Is(err, ErrUnsupportedLayout) {
		t.Errorf("CLI layout: err = %v, want ErrUnsupportedLayout", err)
	}
}
