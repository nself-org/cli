package database

// Purpose: TrackPluginTables tests on a loopback httptest Hasura stub (bulk ops apply in order, one can fail).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

const fixtureSecret = "fixture-admin-secret"

type hasuraStub struct {
	mu      sync.Mutex
	tracked map[string]bool
	reqs    []string   // request type per call
	ops     [][]string // op types per bulk call
	bulks   [][]string // qualified tables per bulk call
	failAt  int        // 1-based index of the bulk op to reject (0 = none)
	status  int        // export_metadata status override
	body    string     // export_metadata body override
}

func newStub(t *testing.T, tracked ...string) (*hasuraStub, *config.Config) {
	t.Helper()
	st := &hasuraStub{tracked: map[string]bool{}}
	for _, k := range tracked {
		st.tracked[k] = true
	}
	srv := httptest.NewServer(http.HandlerFunc(st.serve))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := &config.Config{}
	cfg.Hasura.Port = port
	stubSeams(t, nil)
	return st, cfg
}

// stubSeams swaps the secret reader and the Postgres check for the test.
func stubSeams(t *testing.T, missing map[string]bool) {
	t.Helper()
	oldS, oldE := trackReadSecret, trackTableExists
	trackReadSecret = func(context.Context, *config.Config) (string, error) { return fixtureSecret, nil }
	trackTableExists = func(_ context.Context, _ *config.Config, schema, name string) (bool, error) {
		return !missing[schema+"."+name], nil
	}
	t.Cleanup(func() { trackReadSecret, trackTableExists = oldS, oldE })
}

func (s *hasuraStub) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var req struct {
		Type string          `json:"type"`
		Args json.RawMessage `json:"args"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	s.reqs = append(s.reqs, req.Type)
	if r.Header.Get("X-Hasura-Admin-Secret") != fixtureSecret {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"access denied"}`))
		return
	}
	switch req.Type {
	case "export_metadata":
		if s.status != 0 {
			w.WriteHeader(s.status)
		}
		if s.body != "" {
			_, _ = w.Write([]byte(s.body))
			return
		}
		var tables []map[string]interface{}
		for k := range s.tracked {
			p := strings.SplitN(k, ".", 2)
			tables = append(tables, map[string]interface{}{"table": map[string]string{"schema": p[0], "name": p[1]}})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"sources": []interface{}{map[string]interface{}{"name": "default", "kind": "postgres", "tables": tables}}})
	case "bulk":
		var ops []struct {
			Type string `json:"type"`
			Args struct {
				Source string `json:"source"`
				Table  struct{ Schema, Name string }
			} `json:"args"`
		}
		_ = json.Unmarshal(req.Args, &ops)
		var types, tabs []string
		for _, o := range ops {
			types = append(types, o.Type+"@"+o.Args.Source)
			tabs = append(tabs, o.Args.Table.Schema+"."+o.Args.Table.Name)
		}
		s.ops, s.bulks = append(s.ops, types), append(s.bulks, tabs)
		for i, o := range ops {
			if s.failAt == i+1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprintf(w, `{"path":"$.args[%d]","error":"cannot track table","code":"invalid-configuration"}`, i)
				return
			}
			s.tracked[o.Args.Table.Schema+"."+o.Args.Table.Name] = true
		}
		_, _ = w.Write([]byte(`[]`))
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

// parseTracked stands in for appready.TrackedTables (default source only).
func parseTracked(raw []byte) (map[string]bool, error) {
	var doc struct {
		Sources []struct {
			Name   string
			Tables []struct{ Table struct{ Schema, Name string } }
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("not JSON: %w", err)
	}
	set := map[string]bool{}
	for _, s := range doc.Sources {
		for _, t := range s.Tables {
			if s.Name == "" || s.Name == "default" {
				set[t.Table.Schema+"."+t.Table.Name] = true
			}
		}
	}
	return set, nil
}

func (s *hasuraStub) count(typ string) int {
	n := 0
	for _, r := range s.reqs {
		if r == typ {
			n++
		}
	}
	return n
}

var twoPlugins = []PluginTables{
	{Plugin: "alpha", Tables: []string{"np_a_one", "ext.np_a_two"}},
	{Plugin: "beta", Tables: []string{"np_b_one"}},
}

func TestTrackPluginTablesBulkOnce(t *testing.T) {
	st, cfg := newStub(t, "public.hasura_auth_users")
	res, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"ext.np_a_two", "public.np_a_one", "public.np_b_one"}; !reflect.DeepEqual(res.Tracked, want) {
		t.Fatalf("tracked = %v, want %v", res.Tracked, want)
	}
	if st.count("export_metadata") != 1 || st.count("bulk") != 1 || len(st.reqs) != 2 {
		t.Fatalf("want one export and one bulk, got %v", st.reqs)
	}
	if !st.tracked["public.hasura_auth_users"] {
		t.Fatal("a pre-tracked table must stay tracked")
	}
}

func TestTrackPluginTablesIdempotent(t *testing.T) {
	st, cfg := newStub(t)
	if _, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked); err != nil {
		t.Fatal(err)
	}
	before := len(st.reqs)
	res, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.reqs[before:]; len(res.Tracked) != 0 || len(res.Already) != 3 || len(got) != 1 || got[0] != "export_metadata" {
		t.Fatalf("second run: tracked=%v already=%v reqs=%v, want only the export", res.Tracked, res.Already, got)
	}
}

func TestTrackPluginTablesMissingTableNamed(t *testing.T) {
	st, cfg := newStub(t)
	stubSeams(t, map[string]bool{"public.np_b_one": true})
	_, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	var te *TrackError
	if !errors.As(err, &te) || te.Plugin != "beta" || te.Table != "public.np_b_one" || !strings.Contains(err.Error(), "beta") || !strings.Contains(err.Error(), "public.np_b_one") {
		t.Fatalf("want a *TrackError naming beta/public.np_b_one, got %v", err)
	}
	if st.count("bulk") != 0 {
		t.Fatalf("no track request may be sent when a table is missing, got %v", st.reqs)
	}
}

func TestTrackPluginTablesOnlyTrackOps(t *testing.T) {
	st, cfg := newStub(t, "public.np_a_one")
	if _, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked); err != nil {
		t.Fatal(err)
	}
	if st.count("export_metadata")+st.count("bulk") != len(st.reqs) {
		t.Fatalf("unexpected request type in %v (replace_metadata and friends are forbidden)", st.reqs)
	}
	if len(st.ops) != 1 {
		t.Fatalf("want exactly one bulk request, got %d", len(st.ops))
	}
	for _, o := range st.ops[0] {
		if o != "pg_track_table@default" {
			t.Fatalf("bulk op %q is not pg_track_table on source default", o)
		}
	}
	if want := []string{"ext.np_a_two", "public.np_b_one"}; !reflect.DeepEqual(st.bulks[0], want) {
		t.Fatalf("bulk tables = %v, want only the untracked %v", st.bulks[0], want)
	}
}

func TestTrackPluginTablesAuthErrorNotEmpty(t *testing.T) {
	for name, set := range map[string]func(*hasuraStub){
		"403 json body": func(s *hasuraStub) { s.status, s.body = 403, `{"error":"forbidden","sources":[]}` },
		"401 json body": func(s *hasuraStub) { s.status, s.body = 401, `{}` },
		"200 not json":  func(s *hasuraStub) { s.body = `<html>proxy login</html>` },
	} {
		t.Run(name, func(t *testing.T) {
			st, cfg := newStub(t)
			set(st)
			res, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
			if err == nil {
				t.Fatalf("an unreadable export must be an error, got result %+v", res)
			}
			if strings.Contains(err.Error(), fixtureSecret) {
				t.Fatalf("error leaks the admin secret: %v", err)
			}
			if st.count("bulk") != 0 {
				t.Fatalf("no bulk after a failed export, got %v", st.reqs)
			}
		})
	}
}

func TestTrackPluginTablesPartialFailureRerun(t *testing.T) {
	st, cfg := newStub(t)
	st.failAt = 2
	_, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	var te *TrackError
	if !errors.As(err, &te) || te.Table != "public.np_a_one" || te.Plugin != "alpha" || !strings.Contains(err.Error(), te.Table) {
		t.Fatalf("error must name the rejected table and plugin: %v", err)
	}
	st.failAt = 0
	res, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.bulks[len(st.bulks)-1]; len(res.Tracked) != 2 || len(res.Already) != 1 || len(got) != 2 {
		t.Fatalf("rerun (op 1 stayed applied): tracked=%v already=%v bulk=%v, want the 2 still-missing", res.Tracked, res.Already, got)
	}
}

// TestTrackPluginTablesRejectionMapsFilteredIndex: the bulk path counts the filtered list.
func TestTrackPluginTablesRejectionMapsFilteredIndex(t *testing.T) {
	st, cfg := newStub(t, "public.np_a_one")
	st.failAt = 2
	_, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	var te *TrackError
	if !errors.As(err, &te) || te.Table != "public.np_b_one" || te.Plugin != "beta" || !strings.Contains(err.Error(), "public.np_b_one") {
		t.Fatalf("want the rejection to name beta/public.np_b_one, got %v", err)
	}
}

func TestTrackPluginTablesDedupAcrossPlugins(t *testing.T) {
	st, cfg := newStub(t)
	dup := []PluginTables{{Plugin: "a", Tables: []string{"np_x"}}, {Plugin: "b", Tables: []string{"public.np_x"}}}
	if res, err := TrackPluginTables(context.Background(), cfg, dup, parseTracked); err != nil || len(res.Tracked) != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(st.bulks) != 1 || !reflect.DeepEqual(st.bulks[0], []string{"public.np_x"}) {
		t.Fatalf("bulk tables = %v, want exactly one op", st.bulks)
	}
}

// TestTrackPluginTablesExistenceCheckError: a failing Postgres check names the plugin and table, sends no bulk.
func TestTrackPluginTablesExistenceCheckError(t *testing.T) {
	st, cfg := newStub(t)
	trackTableExists = func(context.Context, *config.Config, string, string) (bool, error) {
		return false, errors.New("psql: container not running")
	}
	_, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	var te *TrackError
	if !errors.As(err, &te) || te.Plugin == "" || te.Table == "" || !strings.Contains(err.Error(), te.Plugin) || !strings.Contains(err.Error(), te.Table) || !strings.Contains(err.Error(), "container not running") {
		t.Fatalf("want an error naming plugin, table and cause, got %v", err)
	}
	if st.count("bulk") != 0 {
		t.Fatalf("no bulk after a failed existence check, got %v", st.reqs)
	}
}
