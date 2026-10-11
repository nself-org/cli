package database

// Purpose: TrackPluginTables retry tests (D-0389) and the default-source rule
// (D-0388), against an httptest Hasura fake with a scripted bulk reply.
// Inputs: none (loopback only, no Docker, no Postgres).
// Outputs: pass/fail.
// Constraints: each test pins the number of export_metadata and bulk requests,
// so removing the single retry makes it fail.

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

// retryStub is a Hasura fake whose bulk reply is scripted per call: onBulk
// receives the 1-based bulk number, the tables sent and the tracked set (it may
// add tables, like a concurrent writer), and returns the failing op index (-1 =
// none) plus the Hasura error code.
type retryStub struct {
	mu      sync.Mutex
	tracked map[string]bool
	exports int
	bulks   [][]string
	onBulk  func(n int, tabs []string, tracked map[string]bool) (int, string)
}

func newRetryStub(t *testing.T) (*retryStub, *config.Config) {
	t.Helper()
	st := &retryStub{tracked: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(st.serve))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := &config.Config{}
	cfg.Hasura.Port = port
	stubSeams(t, nil)
	return st, cfg
}

func (s *retryStub) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var req struct {
		Type string          `json:"type"`
		Args json.RawMessage `json:"args"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	switch req.Type {
	case "export_metadata":
		s.exports++
		var tables []map[string]interface{}
		for k := range s.tracked {
			p := strings.SplitN(k, ".", 2)
			tables = append(tables, map[string]interface{}{"table": map[string]string{"schema": p[0], "name": p[1]}})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"sources": []interface{}{map[string]interface{}{"name": "default", "kind": "postgres", "tables": tables}}})
	case "bulk":
		var ops []struct {
			Args struct{ Table struct{ Schema, Name string } } `json:"args"`
		}
		_ = json.Unmarshal(req.Args, &ops)
		var tabs []string
		for _, o := range ops {
			tabs = append(tabs, o.Args.Table.Schema+"."+o.Args.Table.Name)
		}
		s.bulks = append(s.bulks, tabs)
		idx, code := s.onBulk(len(s.bulks), tabs, s.tracked)
		if idx >= 0 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"path":"$.args[%d]","error":"view/table already tracked: \"%s\"","code":%q}`, idx, tabs[idx], code)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

// trackUpTo marks tabs[:n] tracked, as a bulk applies ops in order until one fails.
func trackUpTo(tracked map[string]bool, tabs []string, n int) {
	for _, tb := range tabs[:n] {
		tracked[tb] = true
	}
}

// First bulk: op 0 applied by us, op 1 tracked by a concurrent writer. The
// retry must send only the table that is still untracked.
func TestTrackPluginTablesRetriesAfterConcurrentTrack(t *testing.T) {
	st, cfg := newRetryStub(t)
	st.onBulk = func(n int, tabs []string, tr map[string]bool) (int, string) {
		if n == 1 {
			trackUpTo(tr, tabs, 1)
			tr[tabs[1]] = true // the concurrent writer
			return 1, "already-tracked"
		}
		trackUpTo(tr, tabs, len(tabs))
		return -1, ""
	}
	res, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	if err != nil {
		t.Fatalf("a concurrent track must be retried, got %v", err)
	}
	if st.exports != 2 || len(st.bulks) != 2 {
		t.Fatalf("want 2 exports and 2 bulks, got %d and %d", st.exports, len(st.bulks))
	}
	if want := []string{"public.np_b_one"}; !reflect.DeepEqual(st.bulks[1], want) {
		t.Fatalf("retry bulk = %v, want only the still-untracked %v", st.bulks[1], want)
	}
	if want := []string{"ext.np_a_two", "public.np_a_one", "public.np_b_one"}; !reflect.DeepEqual(res.Tracked, want) || len(res.Already) != 0 {
		t.Fatalf("result = %+v, want all three tracked and none already", res)
	}
}

// A concurrent writer tracked everything: the re-read finds nothing to do.
func TestTrackPluginTablesRetryFindsNothingLeft(t *testing.T) {
	st, cfg := newRetryStub(t)
	st.onBulk = func(n int, tabs []string, tr map[string]bool) (int, string) {
		trackUpTo(tr, tabs, len(tabs))
		return 0, "already-tracked"
	}
	res, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	if err != nil {
		t.Fatal(err)
	}
	if st.exports != 2 || len(st.bulks) != 1 {
		t.Fatalf("want 2 exports and 1 bulk, got %d and %d", st.exports, len(st.bulks))
	}
	if len(res.Tracked) != 3 || len(res.Already) != 0 {
		t.Fatalf("result = %+v", res)
	}
}

// The retry bulk is rejected too: a *TrackError naming the table, never a third bulk.
func TestTrackPluginTablesRetryFailsOnlyOnce(t *testing.T) {
	st, cfg := newRetryStub(t)
	st.onBulk = func(n int, tabs []string, tr map[string]bool) (int, string) {
		if n == 1 {
			trackUpTo(tr, tabs, 1)
			tr[tabs[1]] = true
		}
		return 0, "already-tracked"
	}
	_, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	var te *TrackError
	if !errors.As(err, &te) || te.Table != "public.np_b_one" || te.Plugin != "beta" || !strings.Contains(err.Error(), "public.np_b_one") {
		t.Fatalf("want a *TrackError naming beta/public.np_b_one, got %v", err)
	}
	if len(st.bulks) != 2 || st.exports != 2 {
		t.Fatalf("want exactly 2 bulks and 2 exports (never a third bulk), got %d and %d", len(st.bulks), st.exports)
	}
}

// Any other Hasura code is not retried: one export, one bulk, the old error.
func TestTrackPluginTablesNoRetryOtherCode(t *testing.T) {
	st, cfg := newRetryStub(t)
	st.onBulk = func(n int, tabs []string, tr map[string]bool) (int, string) { return 1, "invalid-configuration" }
	_, err := TrackPluginTables(context.Background(), cfg, twoPlugins, parseTracked)
	var te *TrackError
	if !errors.As(err, &te) || te.Table != "public.np_a_one" || te.Plugin != "alpha" {
		t.Fatalf("want a *TrackError naming alpha/public.np_a_one, got %v", err)
	}
	if st.exports != 1 || len(st.bulks) != 1 {
		t.Fatalf("want 1 export and 1 bulk, got %d and %d", st.exports, len(st.bulks))
	}
}

// A table listed only under a second source is not tracked on default, so it is
// tracked there (D-0388).
func TestTrackPluginTablesTracksDefaultWhenOnlyOtherSourceHasIt(t *testing.T) {
	st, cfg := newStub(t)
	st.body = `{"sources":[{"name":"default","kind":"postgres","tables":[]},
	 {"name":"other","kind":"postgres","tables":[{"table":{"schema":"public","name":"np_x"}}]}]}`
	plugins := []PluginTables{{Plugin: "a", Tables: []string{"public.np_x"}}}
	res, err := TrackPluginTables(context.Background(), cfg, plugins, parseTracked)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Tracked, []string{"public.np_x"}) || len(res.Already) != 0 {
		t.Fatalf("result = %+v, want np_x tracked", res)
	}
	if len(st.ops) != 1 || !reflect.DeepEqual(st.ops[0], []string{"pg_track_table@default"}) || !reflect.DeepEqual(st.bulks[0], []string{"public.np_x"}) {
		t.Fatalf("bulk ops = %v tables = %v, want pg_track_table@default for public.np_x", st.ops, st.bulks)
	}
}
