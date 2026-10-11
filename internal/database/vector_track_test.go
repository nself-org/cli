package database

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// fakeHasura answers export_metadata from a queue of documents and records
// every request type; bulkStatus/bulkBody answer the bulk call.
type fakeHasura struct {
	mu         sync.Mutex
	exports    []string
	types      []string
	bulks      [][]map[string]any
	bulkStatus int
	bulkBody   string
	secret     string
}

func (f *fakeHasura) serve(t *testing.T) *config.Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.secret = r.Header.Get("X-Hasura-Admin-Secret")
		var req struct {
			Type string          `json:"type"`
			Args json.RawMessage `json:"args"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.types = append(f.types, req.Type)
		switch req.Type {
		case "export_metadata":
			doc := f.exports[0]
			if len(f.exports) > 1 {
				f.exports = f.exports[1:]
			}
			_, _ = w.Write([]byte(doc))
		case "bulk":
			var ops []map[string]any
			_ = json.Unmarshal(req.Args, &ops)
			f.bulks = append(f.bulks, ops)
			if f.bulkStatus != 0 {
				w.WriteHeader(f.bulkStatus)
				_, _ = w.Write([]byte(f.bulkBody))
				return
			}
			_, _ = w.Write([]byte(`[{"message":"success"},{"message":"success"}]`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"code":"unexpected","error":"unexpected request %s"}`, req.Type)
		}
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := &config.Config{}
	cfg.Hasura.Port = port
	cfg.Hasura.AdminSecret = "test-secret"
	return cfg
}

// liveDoc builds an export_metadata document. A select role is "name" (columns
// id and embedding), "name:*" (all columns) or "name:id" (only the listed column).
// Select columns are never read from the role name alone.
func liveDoc(selectRoles []string, fnRoles []string, fnTracked bool) string {
	var sel, fperm []string
	for _, r := range selectRoles {
		r, root, _ := strings.Cut(r, "!") // "name!query", "name!sub", "name!pk"
		extra := ""
		switch root {
		case "query":
			extra = `,"query_root_fields":[]`
		case "sub":
			extra = `,"subscription_root_fields":[]`
		case "pk":
			extra = `,"query_root_fields":["select_by_pk"]`
		case "ok":
			extra = `,"query_root_fields":["select","select_by_pk"],"subscription_root_fields":["select"]`
		}
		role, cols, _ := strings.Cut(r, ":")
		switch cols {
		case "":
			cols = `["id","embedding"]`
		case "*":
			cols = `"*"`
		default:
			cols = fmt.Sprintf("[%q]", cols)
		}
		sel = append(sel, fmt.Sprintf(`{"role":%q,"permission":{"columns":%s,"filter":{}%s}}`, role, cols, extra))
	}
	for _, r := range fnRoles {
		fperm = append(fperm, fmt.Sprintf(`{"role":%q}`, r))
	}
	fn := ""
	if fnTracked {
		fn = fmt.Sprintf(`,"functions":[{"function":{"schema":"public","name":"docs_nearest_embedding"},"permissions":[%s]}]`, strings.Join(fperm, ","))
	}
	return fmt.Sprintf(`{"version":3,"sources":[{"name":"default","kind":"postgres","tables":[{"table":{"schema":"public","name":"docs"},"select_permissions":[%s]}]%s}]}`,
		strings.Join(sel, ","), fn)
}

func opTypes(ops []map[string]any) []string {
	var out []string
	for _, o := range ops {
		out = append(out, o["type"].(string))
	}
	return out
}

func TestTrackFunction(t *testing.T) {
	track := func(f *fakeHasura, roles ...string) error {
		return TrackFunction(context.Background(), f.serve(t), "public", "docs_nearest_embedding", "docs", "embedding", roles)
	}

	t.Run("tracks the function and a permission per role in one bulk", func(t *testing.T) {
		f := &fakeHasura{exports: []string{liveDoc([]string{"user", "staff"}, nil, false)}}
		if err := track(f, "user", "staff"); err != nil {
			t.Fatal(err)
		}
		if len(f.bulks) != 1 {
			t.Fatalf("want exactly one bulk call, got %d", len(f.bulks))
		}
		got := strings.Join(opTypes(f.bulks[0]), ",")
		if got != "pg_track_function,pg_create_function_permission,pg_create_function_permission" {
			t.Errorf("bulk ops = %s", got)
		}
		args := f.bulks[0][0]["args"].(map[string]any)
		fn := args["function"].(map[string]any)
		if args["source"] != "default" || fn["schema"] != "public" || fn["name"] != "docs_nearest_embedding" {
			t.Errorf("pg_track_function args = %v", args)
		}
		if perm := f.bulks[0][1]["args"].(map[string]any); perm["role"] != "user" {
			t.Errorf("first permission args = %v", perm)
		}
		if f.secret != "test-secret" {
			t.Errorf("admin secret header = %q", f.secret)
		}
	})

	t.Run("never replaces or applies metadata", func(t *testing.T) {
		f := &fakeHasura{exports: []string{liveDoc([]string{"user"}, nil, false)}}
		if err := track(f, "user"); err != nil {
			t.Fatal(err)
		}
		for _, typ := range f.types {
			if typ != "export_metadata" && typ != "bulk" {
				t.Errorf("unexpected metadata request %q", typ)
			}
		}
	})

	t.Run("already tracked with every role sends nothing", func(t *testing.T) {
		f := &fakeHasura{exports: []string{liveDoc([]string{"user"}, []string{"user"}, true)}}
		if err := track(f, "user"); err != nil {
			t.Fatal(err)
		}
		if len(f.bulks) != 0 {
			t.Errorf("idempotent rerun sent %v", f.bulks)
		}
	})

	t.Run("completes a partial state with only the missing permission", func(t *testing.T) {
		f := &fakeHasura{exports: []string{liveDoc([]string{"user", "staff"}, []string{"user"}, true)}}
		if err := track(f, "user", "staff"); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(opTypes(f.bulks[0]), ","); got != "pg_create_function_permission" {
			t.Errorf("bulk ops = %s", got)
		}
		if f.bulks[0][0]["args"].(map[string]any)["role"] != "staff" {
			t.Errorf("wrong role granted: %v", f.bulks[0][0])
		}
	})

	t.Run("a race on already-tracked is success after re-planning", func(t *testing.T) {
		f := &fakeHasura{
			exports:    []string{liveDoc([]string{"user"}, nil, false), liveDoc([]string{"user"}, []string{"user"}, true)},
			bulkStatus: 400,
			bulkBody:   `{"code":"already-tracked","error":"view/table already tracked","path":"$.args[0].args"}`,
		}
		if err := track(f, "user"); err != nil {
			t.Fatalf("race on already-tracked: %v", err)
		}
		if len(f.bulks) != 1 {
			t.Errorf("bulk calls = %d, want 1", len(f.bulks))
		}
	})

	t.Run("E514 when a role has no select permission", func(t *testing.T) {
		f := &fakeHasura{exports: []string{liveDoc([]string{"user"}, nil, false)}}
		err := track(f, "user", "guest")
		if err == nil || !strings.Contains(err.Error(), "E514") || !strings.Contains(err.Error(), "guest") {
			t.Fatalf("err = %v, want E514 naming guest", err)
		}
		if len(f.bulks) != 0 {
			t.Error("sent a bulk although a role lacks select")
		}
	})

	t.Run("E514 when a --roles role hides the vector column", func(t *testing.T) {
		f := &fakeHasura{exports: []string{liveDoc([]string{"user", "staff:id"}, nil, false)}}
		err := track(f, "user", "staff")
		if err == nil || !strings.Contains(err.Error(), "E514") || !strings.Contains(err.Error(), "staff") || !strings.Contains(err.Error(), "embedding") {
			t.Fatalf("err = %v, want E514 naming staff and the column", err)
		}
		if len(f.bulks) != 0 {
			t.Error("sent a bulk although a role hides the column")
		}
	})

	t.Run("E514 when any select role hides the column, even outside --roles", func(t *testing.T) {
		f := &fakeHasura{exports: []string{liveDoc([]string{"user", "viewer:id"}, nil, false)}}
		err := track(f, "user")
		if err == nil || !strings.Contains(err.Error(), "E514") || !strings.Contains(err.Error(), "viewer") || strings.Contains(err.Error(), "user,") {
			t.Fatalf("err = %v, want E514 naming only viewer", err)
		}
		if len(f.bulks) != 0 {
			t.Error("sent a bulk although a select role hides the column")
		}
	})

	for _, root := range []string{"query", "sub", "pk"} {
		t.Run("E514 for a root-less select permission ("+root+")", func(t *testing.T) {
			f := &fakeHasura{exports: []string{liveDoc([]string{"user", "rootless:*!" + root}, nil, false)}}
			err := track(f, "user")
			if err == nil || !strings.Contains(err.Error(), "E514") || !strings.Contains(err.Error(), "rootless") || !strings.Contains(err.Error(), "not exposed at the root") {
				t.Fatalf("err = %v, want E514 naming rootless and the root reason", err)
			}
			if len(f.bulks) != 0 {
				t.Error("sent a bulk although a role is root-less")
			}
		})
	}

	t.Run("root fields that keep select pass", func(t *testing.T) {
		f := &fakeHasura{exports: []string{liveDoc([]string{"user!ok"}, nil, false)}}
		if err := track(f, "user"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("star columns and a hidden-role fix pass", func(t *testing.T) {
		f := &fakeHasura{exports: []string{liveDoc([]string{"user:*", "staff"}, nil, false)}}
		if err := track(f, "user", "staff"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("persistent already-tracked race is an error, never nil", func(t *testing.T) {
		f := &fakeHasura{
			exports:    []string{liveDoc([]string{"user"}, nil, false)},
			bulkStatus: 400,
			bulkBody:   `{"code":"already-tracked","error":"x","path":"$.args[0].args"}`,
		}
		if err := track(f, "user"); err == nil || !strings.Contains(err.Error(), "still lacks") {
			t.Fatalf("err = %v, want an error naming what is missing", err)
		}
	})

	t.Run("E513 when the table is not tracked", func(t *testing.T) {
		f := &fakeHasura{exports: []string{`{"version":3,"sources":[{"name":"default","tables":[]}]}`}}
		if err := track(f, "user"); err == nil || !strings.Contains(err.Error(), "E513") {
			t.Fatalf("err = %v, want E513", err)
		}
	})

	t.Run("E513 when the function does not exist in Postgres", func(t *testing.T) {
		f := &fakeHasura{
			exports:    []string{liveDoc([]string{"user"}, nil, false)},
			bulkStatus: 400,
			bulkBody:   `{"code":"not-exists","error":"no such function exists in postgres : \"public.docs_nearest_embedding\"","path":"$.args[0].args"}`,
		}
		err := track(f, "user")
		if err == nil || !strings.Contains(err.Error(), "E513") || !strings.Contains(err.Error(), "migrate") {
			t.Fatalf("err = %v, want E513 with the migrate hint", err)
		}
	})

	t.Run("other Hasura failures are plain errors", func(t *testing.T) {
		f := &fakeHasura{exports: []string{liveDoc([]string{"user"}, nil, false)}, bulkStatus: 500, bulkBody: `{"error":"boom"}`}
		err := track(f, "user")
		if err == nil || strings.Contains(err.Error(), "E513") || strings.Contains(err.Error(), "E514") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestMetadataTracksTable(t *testing.T) {
	write := func(dir, rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sources := "- name: default\n  tables:\n    - table:\n        name: docs\n        schema: public\n"
	cases := []struct {
		name, rel, content string
		present, tracked   bool
	}{
		{"no metadata", "", "", false, false},
		{"export layout", "hasura/metadata/sources.yaml", sources, true, true},
		{"export layout, other table", "hasura/metadata/sources.yaml", strings.ReplaceAll(sources, "docs", "notes"), true, false},
		{"flat tables.yaml", "hasura/metadata/tables.yaml", "- table:\n    name: docs\n    schema: public\n", true, true},
		{"legacy json", "hasura/metadata.json", `{"sources":[{"name":"default","tables":[{"table":{"schema":"public","name":"docs"}}]}]}`, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.rel != "" {
				write(dir, tc.rel, tc.content)
			}
			present, tracked, err := MetadataTracksTable(dir, "public", "docs")
			if err != nil || present != tc.present || tracked != tc.tracked {
				t.Fatalf("got present=%v tracked=%v err=%v", present, tracked, err)
			}
		})
	}
}

func TestCheckDiskColumnVisible(t *testing.T) {
	// perm is the permission body; each layout wraps it the way its reader expects.
	layouts := map[string]func(perm string) (string, string){
		"sources.yaml": func(perm string) (string, string) {
			return "hasura/metadata/sources.yaml", "- name: default\n  tables:\n    - table: {name: docs, schema: public}\n      select_permissions:\n        - role: user\n          permission: {" + perm + "}\n"
		},
		"tables.yaml": func(perm string) (string, string) {
			return "hasura/metadata/tables.yaml", "- table: {name: docs, schema: public}\n  select_permissions:\n    - role: user\n      permission: {" + perm + "}\n"
		},
		"metadata.json": func(perm string) (string, string) {
			return "hasura/metadata.json", `{"sources":[{"name":"default","tables":[{"table":{"schema":"public","name":"docs"},"select_permissions":[{"role":"user","permission":{` + perm + `}}]}]}]}`
		},
	}
	for layout, build := range layouts {
		cases := map[string]bool{
			`"columns":["id","embedding"],"filter":{}`:                                false,
			`"columns":"*","filter":{}`:                                               false,
			`"columns":["id"],"filter":{}`:                                            true,
			`"columns":[],"filter":{}`:                                                true,
			`"columns":"*","filter":{},"query_root_fields":[]`:                        true,
			`"columns":"*","filter":{},"subscription_root_fields":[]`:                 true,
			`"columns":"*","filter":{},"query_root_fields":["select_by_pk"]`:          true,
			`"columns":"*","filter":{},"query_root_fields":["select","select_by_pk"]`: false,
		}
		for perm, wantErr := range cases {
			dir := t.TempDir()
			rel, body := build(perm)
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			err := CheckDiskColumnVisible(dir, "public", "docs", "embedding")
			if wantErr != (err != nil) || (err != nil && !strings.Contains(err.Error(), "E514")) {
				t.Errorf("%s %s: err = %v, want error=%v", layout, perm, err, wantErr)
			}
			if wantErr && strings.Contains(perm, "root_fields") && (err == nil || !strings.Contains(err.Error(), "not exposed at the root")) {
				t.Errorf("%s %s: err = %v, want the root reason", layout, perm, err)
			}
		}
	}
	if err := CheckDiskColumnVisible(t.TempDir(), "public", "docs", "embedding"); err != nil {
		t.Errorf("no metadata must pass here (the tracked check reports it): %v", err)
	}
}
