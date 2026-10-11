package database

// Purpose: unit tests for TrackPluginTables against an httptest Hasura stub
// (no network beyond loopback, no Docker, no Postgres).
// Constraints: the stub keeps the tracked set, applies bulk ops in order and can
// fail one op after applying the ones before it (non-atomic bulk).

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

// parseTracked is the test stand-in for appready.TrackedTables.
func parseTracked(raw []byte) (map[string]bool, error) {
	var doc struct {
		Sources []struct {
			Tables []struct {
				Table struct{ Schema, Name string } `json:"table"`
			} `json:"tables"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("export_metadata response is not JSON: %w", err)
	}
	set := map[string]bool{}
	for _, s := range doc.Sources {
		for _, t := range s.Tables {
			set[t.Table.Schema+"."+t.Table.Name] = true
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
	if len(res.Tracked) != 0 || len(res.Already) != 3 {
		t.Fatalf("second run: tracked=%v already=%v", res.Tracked, res.Already)
	}
	if got := st.reqs[before:]; len(got) != 1 || got[0] != "export_metadata" {
		t.Fatalf("second run must send only the export, got %v", got)
	}
}

func TestTrackPluginTablesMissingTableNamed(t *testing.T) {
	st, cfg := newStub(t)
	stubSeams(t, map[string]bool{"public.np_b_one": true})
	_, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	if err == nil {
		t.Fatal("want an error for a table missing from Postgres")
	}
	if !strings.Contains(err.Error(), "beta") || !strings.Contains(err.Error(), "public.np_b_one") {
		t.Fatalf("error must name plugin and table: %v", err)
	}
	var te *TrackError
	if !errors.As(err, &te) || te.Plugin != "beta" || te.Table != "public.np_b_one" {
		t.Fatalf("want a *TrackError for beta/public.np_b_one, got %#v", err)
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
	for _, r := range st.reqs {
		if r != "export_metadata" && r != "bulk" {
			t.Fatalf("unexpected request type %q (replace_metadata and friends are forbidden)", r)
		}
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
	if err == nil {
		t.Fatal("want the op-2 rejection as an error")
	}
	var te *TrackError
	if !errors.As(err, &te) || te.Table != "public.np_a_one" || te.Plugin != "alpha" {
		t.Fatalf("error must name the rejected table and plugin: %v", err)
	}
	if !strings.Contains(err.Error(), te.Table) {
		t.Fatalf("message must contain the rejected table %s: %v", te.Table, err)
	}
	if len(st.tracked) != 1 {
		t.Fatalf("op 1 should stay applied after the failure, tracked=%v", st.tracked)
	}
	st.failAt = 0
	res, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tracked) != 2 || len(res.Already) != 1 {
		t.Fatalf("rerun: tracked=%v already=%v", res.Tracked, res.Already)
	}
	if got := st.bulks[len(st.bulks)-1]; len(got) != 2 {
		t.Fatalf("rerun must send only the 2 still-missing tables, got %v", got)
	}
}

func TestQualifyPluginTable(t *testing.T) {
	if QualifyPluginTable("x") != "public.x" || QualifyPluginTable("s.x") != "s.x" {
		t.Fatal("qualification rule changed")
	}
}
