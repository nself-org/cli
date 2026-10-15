package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func vsDoc(perms string, fns string) []byte {
	return []byte(`{"version":3,"sources":[{"name":"default","tables":[{"table":{"schema":"public","name":"docs"},"select_permissions":[` +
		perms + `]}],"functions":[` + fns + `]}]}`)
}

const (
	vsFn     = `{"function":{"schema":"public","name":"docs_nearest_embedding"},"permissions":[{"role":"user"}]}`
	vsSafe   = `{"role":"user","permission":{"columns":"*"}}`
	vsHiding = `{"role":"viewer","permission":{"columns":["id","owner"]}}`
)

// TestVectorSearchResults covers D-0339 at the doctor surface.
func TestVectorSearchResults(t *testing.T) {
	for _, tc := range []struct {
		name     string
		live     []byte
		inferred bool
		status   string // "" = no result
		want     []string
	}{
		{"none tracked", vsDoc(vsSafe, ""), true, "", nil},
		{"safe", vsDoc(vsSafe, vsFn), true, "pass", []string{"1 tracked vector-search function(s)"}},
		{"hidden column added later", vsDoc(vsSafe+","+vsHiding, vsFn), true, "fail",
			[]string{"E514", "public.docs_nearest_embedding", `viewer: hides column "embedding"`}},
		{"inference off: viewer cannot call it", vsDoc(vsSafe+","+vsHiding, vsFn), false, "pass", nil},
		{"unreadable metadata", []byte("{not json"), true, "skip", []string{"not checked"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := vectorSearchResults(tc.live, tc.inferred)
			if tc.status == "" {
				if len(res) != 0 {
					t.Fatalf("want no result, got %+v", res)
				}
				return
			}
			if len(res) != 1 || res[0].Status != tc.status || res[0].Section != "hasura" {
				t.Fatalf("results = %+v, want one %s", res, tc.status)
			}
			for _, w := range tc.want {
				if !strings.Contains(res[0].Message, w) {
					t.Errorf("message lacks %q: %s", w, res[0].Message)
				}
			}
			if tc.status == "fail" && (res[0].Name != "VECTOR-SEARCH-public.docs_nearest_embedding" || !strings.Contains(res[0].FixCmd, "--track")) {
				t.Errorf("fail result = %+v", res[0])
			}
		})
	}
}

// stubExport replaces the live metadata read for one test.
func stubExport(t *testing.T, live []byte, err error) {
	t.Helper()
	saved := exportLiveMetadata
	t.Cleanup(func() { exportLiveMetadata = saved })
	exportLiveMetadata = func(context.Context, *config.Config) ([]byte, error) { return live, err }
}

// TestVectorSearchChecksLive: no project gives nothing; an unreachable Hasura
// is a skip only when disk names a search function (never a silent pass where
// one is known), and silent otherwise; a reachable one is audited; the
// project's desired inference setting cannot prove the live setting.
func TestVectorSearchChecksLive(t *testing.T) {
	if res := VectorSearchChecks(context.Background(), ""); res != nil {
		t.Fatalf("no project: %+v", res)
	}
	t.Setenv(inferFunctionPerms, "")
	dir := t.TempDir()
	stubExport(t, nil, errors.New("connection refused"))
	if res := VectorSearchChecks(context.Background(), dir); len(res) != 0 {
		t.Fatalf("stack down, no search function on disk: %+v", res)
	}
	meta := filepath.Join(dir, "hasura", "metadata", "databases", "default")
	if err := os.MkdirAll(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "functions.yaml"), []byte("- function:\n    schema: public\n    name: docs_nearest_embedding\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := VectorSearchChecks(context.Background(), dir)
	if len(res) != 1 || res[0].Status != "skip" || !strings.Contains(res[0].Message, "not checked") || !strings.Contains(res[0].Message, "connection refused") {
		t.Fatalf("stack down, search function on disk: %+v", res)
	}

	stubExport(t, vsDoc(vsSafe+","+vsHiding, vsFn), nil)
	if res := VectorSearchChecks(context.Background(), dir); len(res) != 1 || res[0].Status != "fail" {
		t.Fatalf("live unsafe function: %+v", res)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fx\nENV=dev\n"+inferFunctionPerms+"=false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := VectorSearchChecks(context.Background(), dir); len(res) != 1 || res[0].Status != "fail" {
		t.Fatalf("unconfirmed live inference must audit inferred callers: %+v", res)
	}
	if v, ok := os.LookupEnv(inferFunctionPerms); ok && v != "" {
		t.Errorf("the check leaked %s=%q into the process environment", inferFunctionPerms, v)
	}
}

func TestDiskNamesSearchFunction(t *testing.T) {
	dir := t.TempDir()
	if diskNamesSearchFunction(dir) {
		t.Fatal("empty project")
	}
	if err := os.MkdirAll(filepath.Join(dir, "hasura"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hasura", "metadata.json"), []byte(`{"sources":[{"functions":[{"function":{"name":"docs_nearest_embedding"}}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !diskNamesSearchFunction(dir) {
		t.Fatal("metadata.json names a search function")
	}
}
