package verify

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/portable"
)

func TestBundleRetainsTrackedFunction(t *testing.T) {
	w, err := portable.NewWriter(filepath.Join(t.TempDir(), "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.WriteFile("hasura/metadata.json", strings.NewReader(`{"sources":[{"name":"default","functions":[{"function":{"schema":"public","name":"search"}}]}]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Finish(portable.Manifest{DB: portable.DB{Schemas: []string{"public"}}}); err != nil {
		t.Fatal(err)
	}
	r, err := portable.Open(w.Dir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ReadMetadata(r)
	if err != nil || len(meta) != 1 {
		t.Fatalf("bundle functions: %v, %v", meta, err)
	}
}

func TestTrackedFunctionVerification(t *testing.T) {
	want, err := ParseMetadata([]byte(`{"sources":[{"name":"default","tables":[{"table":{"schema":"public","name":"search"}}],"functions":[{"function":{"schema":"public","name":"search"},"configuration":{"exposed_as":"query"},"permissions":[{"role":"user"}]}]}]}`))
	if err != nil || len(want) != 2 {
		t.Fatalf("table and function must both survive: %v, %v", want, err)
	}
	for _, tc := range []struct{ name, field, kind string }{
		{"missing", "", "function_not_tracked"},
		{"configuration", "configuration", "function_configuration"},
		{"permissions", "permissions", "function_permission"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Metadata{}
			for id, record := range want {
				copy := map[string]any{}
				for k, v := range record {
					copy[k] = v
				}
				if record["function"] != nil {
					if tc.field == "" {
						continue
					}
					delete(copy, tc.field)
				}
				got[id] = copy
			}
			mm := CompareState(State{Metadata: got}, portable.Manifest{}, nil, want)
			if len(mm) != 1 || mm[0].Kind != tc.kind {
				t.Fatalf("mismatches: %+v", mm)
			}
		})
	}
	if mm := CompareState(State{Metadata: want}, portable.Manifest{}, nil, want); len(mm) != 0 {
		t.Fatal(mm)
	}
}

func TestFunctionMetadataExportNormalization(t *testing.T) {
	want, err := ParseMetadata([]byte(`{"sources":[{"name":"default","functions":[{"function":{"schema":"public","name":"search"},"permissions":[{"role":"writer"},{"role":"reader"}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseMetadata([]byte(`{"sources":[{"name":"default","functions":[{"function":{"schema":"public","name":"search"},"configuration":{},"permissions":[{"role":"reader"},{"role":"writer"}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if mm := CompareState(State{Metadata: got}, portable.Manifest{}, nil, want); len(mm) != 0 {
		t.Fatal(mm)
	}
	for _, raw := range []string{`{"sources":[{"name":"default","functions":[{}]}]}`, `{"sources":[{"name":"default","functions":[{"function":{"schema":"","name":"search"}}]}]}`} {
		if _, err := ParseMetadata([]byte(raw)); err == nil {
			t.Fatal("malformed function accepted")
		}
	}
}
