package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func vsDoc(perms string, fns string) []byte {
	return []byte(`{"version":3,"sources":[{"name":"default","tables":[{"table":{"schema":"public","name":"docs"},"select_permissions":[` +
		perms + `]}],"functions":[` + fns + `]}]}`)
}

// TestVectorSearchResults covers D-0339 at the doctor surface.
func TestVectorSearchResults(t *testing.T) {
	const fn = `{"function":{"schema":"public","name":"docs_nearest_embedding"}}`
	const safe = `{"role":"user","permission":{"columns":"*"}}`
	for _, tc := range []struct {
		name   string
		live   []byte
		status string // "" = no result
		want   []string
	}{
		{"none tracked", vsDoc(safe, ""), "", nil},
		{"safe", vsDoc(safe, fn), "pass", []string{"1 tracked vector-search function(s)"}},
		{"hidden column added later", vsDoc(safe+`,{"role":"viewer","permission":{"columns":["id","owner"]}}`, fn), "fail",
			[]string{"E514", "public.docs_nearest_embedding", `viewer: hides column "embedding"`}},
		{"unreadable metadata", []byte("{not json"), "skip", []string{"not checked"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := vectorSearchResults(tc.live)
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

// TestVectorSearchChecksNoProject: no project dir gives nothing, and an
// unreachable Hasura is reported as a skip, never a silent pass.
func TestVectorSearchChecksNoProject(t *testing.T) {
	if res := VectorSearchChecks(context.Background(), ""); res != nil {
		t.Fatalf("no project: %+v", res)
	}
	saved := exportLiveMetadata
	t.Cleanup(func() { exportLiveMetadata = saved })
	dir := t.TempDir()
	exportLiveMetadata = func(context.Context, *config.Config) ([]byte, error) {
		return nil, errors.New("connection refused")
	}
	res := VectorSearchChecks(context.Background(), dir)
	if len(res) != 1 || res[0].Status != "skip" || !strings.Contains(res[0].Message, "not checked") || !strings.Contains(res[0].Message, "connection refused") {
		t.Fatalf("unreachable Hasura: %+v", res)
	}
	exportLiveMetadata = func(context.Context, *config.Config) ([]byte, error) {
		return vsDoc(`{"role":"v","permission":{"columns":["id"]}}`, `{"function":{"schema":"public","name":"docs_nearest_embedding"}}`), nil
	}
	if res := VectorSearchChecks(context.Background(), dir); len(res) != 1 || res[0].Status != "fail" {
		t.Fatalf("live unsafe function: %+v", res)
	}
}
