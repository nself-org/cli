package hasurameta

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/portable"
)

const secret = "fixture-admin-secret-value"

func TestBase(t *testing.T) {
	ok := map[string]string{
		"https://p.hasura.app":                "https://p.hasura.app",
		"https://p.hasura.app/v1/graphql":     "https://p.hasura.app",
		"https://p.hasura.app/prefix/":        "https://p.hasura.app/prefix",
		"http://127.0.0.1:8080/v1/graphql":    "http://127.0.0.1:8080",
		"http://localhost:8080":               "http://localhost:8080",
		"  https://p.hasura.app/v1/graphql  ": "https://p.hasura.app",
	}
	for in, want := range ok {
		u, err := Base(in)
		if err != nil || u.String() != want {
			t.Errorf("Base(%q) = %v, %v; want %s", in, u, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x", "http://example.com", "https://u:p@example.com", "https://example.com?a=b", "https://example.com#f", "not a url", "https://"} {
		if _, err := Base(bad); err == nil {
			t.Errorf("Base(%q) accepted", bad)
		} else if strings.Contains(err.Error(), "example.com") {
			t.Errorf("error names the URL: %v", err)
		}
	}
}

func TestExportSendsSecretOnlyInHeader(t *testing.T) {
	var gotHeader, gotBody, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader, gotPath = r.Header.Get("X-Hasura-Admin-Secret"), r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		if r.URL.Path == "/v1/version" {
			_, _ = w.Write([]byte(`{"version":"v2.40.0"}`))
			return
		}
		if gotHeader != secret {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"` + gotHeader + `"}`))
			return
		}
		_, _ = w.Write([]byte(`{"version":3,"sources":[]}`))
	}))
	defer srv.Close()
	base, _ := Base(srv.URL)
	raw, err := Export(context.Background(), base, secret)
	if err != nil || !strings.Contains(string(raw), `"sources"`) || gotPath != "/v1/metadata" || gotHeader != secret || strings.Contains(gotBody, secret) || !strings.Contains(gotBody, "export_metadata") {
		t.Fatalf("export: %s %v path=%s body=%s", raw, err, gotPath, gotBody)
	}
	if v, err := Version(context.Background(), base); err != nil || v != "v2.40.0" {
		t.Fatalf("version %q %v", v, err)
	}
	_, err = Export(context.Background(), base, "wrong-secret-value")
	if err == nil || strings.Contains(err.Error(), "wrong-secret-value") || !strings.Contains(err.Error(), "E517") {
		t.Fatalf("wrong secret must fail with E517 and no echo: %v", err)
	}
}

const rawMetadata = `{"resource_version":7,"metadata":{"version":3,
 "remote_schemas":[{"name":"r"}],"actions":[{"name":"a"},{"name":"b"}],
 "sources":[
  {"name":"other","kind":"postgres","tables":[{"table":{"schema":"public","name":"notes"}}]},
  {"name":"default","kind":"postgres","configuration":{"connection_info":{"database_url":"postgres://u:pw@h/db"}},
   "tables":[
    {"table":{"schema":"public","name":"notes"},"object_relationships":[{"name":"o"}],"array_relationships":[{"name":"x"},{"name":"y"}],
     "select_permissions":[{"role":"user"},{"role":"admin"}],"insert_permissions":[{"role":"user"}],
     "event_triggers":[{"name":"t","webhook":"https://hook.invalid/secret-token"}],"surprise_key":{"k":"v"}},
    {"table":{"schema":"public","name":"dropped"}},
    {"table":{"schema":"auth","name":"users"}}],
   "functions":[{"function":{"schema":"public","name":"search"},"configuration":{"session_argument":"s"},"permissions":[{"role":"user"}],"fn_secret":"https://hook.invalid/fn-token"},{"function":{"schema":"other","name":"f"}},{"function":{"schema":"public","name":"np_hidden"}}]}]}}`

func TestFilterKeepsOnlyBundleTablesAndAllowlistedKeys(t *testing.T) {
	db := portable.DB{Schemas: []string{"public"}, Tables: []portable.Table{{Schema: "public", Name: "notes"}}}
	out, counts, exempt, err := Filter([]byte(rawMetadata), db)
	if err != nil {
		t.Fatal(err)
	}
	want := portable.Hasura{Tables: 1, Relationships: 3, Permissions: 3, Functions: 1, Skipped: portable.HasuraSkipped{RemoteSchemas: 1, Actions: 2}}
	if counts != want {
		t.Fatalf("counts %+v want %+v", counts, want)
	}
	ids := map[string]string{}
	for _, e := range exempt {
		if e.Kind != "metadata" || e.Reason == "" {
			t.Errorf("exemption %+v", e)
		}
		ids[e.ID] = e.Reason
	}
	for _, id := range []string{"source other", "public.dropped", "auth.users", "function other.f"} {
		if ids[id] == "" {
			t.Errorf("dropped item %q is not listed as an exemption: %v", id, ids)
		}
	}
	if len(exempt) != 4 {
		t.Errorf("exemptions %v", exempt)
	}
	for _, leak := range []string{"event_triggers", "hook.invalid", "database_url", "surprise_key", "fn-token", "postgres://", "dropped", "auth", "np_hidden"} {
		if strings.Contains(string(out), leak) {
			t.Errorf("output leaks %q:\n%s", leak, out)
		}
	}
	var doc struct {
		Sources []struct {
			Name      string           `json:"name"`
			Tables    []map[string]any `json:"tables"`
			Functions []map[string]any `json:"functions"`
		} `json:"sources"`
	}
	if err = json.Unmarshal(out, &doc); err != nil || len(doc.Sources) != 1 || doc.Sources[0].Name != "default" || len(doc.Sources[0].Tables) != 1 || len(doc.Sources[0].Functions) != 1 {
		t.Fatalf("shape: %v %s", err, out)
	}
	again, _, _, err := Filter([]byte(rawMetadata), db)
	if err != nil || !bytes.Equal(out, again) {
		t.Fatal("filter output is not deterministic")
	}
	// The unversioned export (no wrapper) is accepted too.
	var wrapped map[string]any
	_ = json.Unmarshal([]byte(rawMetadata), &wrapped)
	inner, _ := json.Marshal(wrapped["metadata"])
	if _, c, _, err := Filter(inner, db); err != nil || c != want {
		t.Fatalf("unwrapped export: %+v %v", c, err)
	}
	for _, bad := range []string{`not json`, `{}`, `{"sources":"x"}`, `{"sources":[]}`, `{"sources":[{"name":"other"}]}`, `{"sources":["x"]}`, `{"sources":[{"name":"default","tables":["x"]}]}`, `{"sources":[{"name":"default","tables":[{"x":1}]}]}`, `{"sources":[{"name":"default","tables":[],"functions":[{"x":1}]}]}`, `{"sources":[{"name":"default","tables":[],"functions":["x"]}]}`} {
		if _, _, _, err := Filter([]byte(bad), db); err == nil {
			t.Errorf("Filter(%q) accepted", bad)
		}
	}
}

// fakePG writes a sealed bundle the way the postgres source does.
type fakePG struct{ fail bool }

func (fakePG) Name() string               { return "postgres" }
func (fakePG) Flags() []importer.FlagSpec { return nil }
func (fakePG) Detect(context.Context, importer.Input) (importer.Detection, error) {
	return importer.Detection{}, nil
}
func (f fakePG) Extract(_ context.Context, _ importer.Input, stage string) (importer.Extracted, error) {
	if f.fail {
		return importer.Extracted{}, os.ErrInvalid
	}
	w, err := portable.NewWriter(stage)
	if err != nil {
		return importer.Extracted{}, err
	}
	if _, err = w.WriteFile("db/data.dump", strings.NewReader("dump-bytes")); err != nil {
		return importer.Extracted{}, err
	}
	m := portable.Manifest{Producer: portable.Producer{Tool: "nself", Version: "1", Source: "postgres"}, DB: portable.DB{Schemas: []string{"public"}, Tables: []portable.Table{{Schema: "public", Name: "notes", Rows: 1, Hash: "1", PK: []string{"id"}}}}}
	_, err = w.Finish(m)
	return importer.Extracted{Stage: stage}, err
}

func metaServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Hasura-Admin-Secret") != secret {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(rawMetadata))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestExtractRecomposesBundleAndRemovesIntermediate(t *testing.T) {
	srv := metaServer(t)
	base, _ := Base(srv.URL)
	stage := filepath.Join(t.TempDir(), "run", "stage")
	extra := func(w *portable.Writer, m *portable.Manifest) error {
		_, err := w.WriteFile("config/extra.txt", strings.NewReader("extra"))
		m.Compat = append(m.Compat, "extra")
		return err
	}
	out, err := Extract(context.Background(), fakePG{}, importer.Input{}, stage, "hasura-cloud", base, secret, extra)
	if err != nil || out.Stage != stage {
		t.Fatalf("extract: %v %+v", err, out)
	}
	r, err := portable.Open(stage)
	if err != nil {
		t.Fatalf("bundle does not open: %v", err)
	}
	m := r.Manifest()
	if m.Producer.Source != "hasura-cloud" || m.Hasura.Tables != 1 || m.Hasura.Relationships != 3 || len(m.Compat) != 1 {
		t.Fatalf("manifest %+v", m)
	}
	have := map[string]bool{}
	for _, f := range m.Files {
		have[f.Path] = true
	}
	for _, p := range []string{"db/data.dump", "hasura/metadata.json", "config/extra.txt"} {
		if !have[p] {
			t.Errorf("member %s missing from %v", p, have)
		}
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(stage), "stage-pg")); !os.IsNotExist(err) {
		t.Fatalf("intermediate postgres bundle left behind: %v", err)
	}
}

func TestExtractFailuresLeaveNoIntermediateAndDoNotDumpOnBadSecret(t *testing.T) {
	srv := metaServer(t)
	base, _ := Base(srv.URL)
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	if _, err := Extract(context.Background(), fakePG{}, importer.Input{}, stage, "nhost", base, "wrong-secret-value", nil); err == nil || strings.Contains(err.Error(), "wrong-secret-value") {
		t.Fatalf("bad secret: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "stage-pg")); !os.IsNotExist(err) {
		t.Fatal("a bad secret must fail before the postgres extract runs")
	}
	if _, err := Extract(context.Background(), fakePG{fail: true}, importer.Input{}, stage, "nhost", base, secret, nil); err == nil {
		t.Fatal("postgres extract failure swallowed")
	}
	if _, err := os.Stat(filepath.Join(root, "stage-pg")); !os.IsNotExist(err) {
		t.Fatal("intermediate left after failure")
	}
	if err := Recompose(filepath.Join(root, "missing"), filepath.Join(root, "out"), "nhost", nil); err == nil {
		t.Fatal("recompose of a missing bundle must fail")
	}
}

func TestRedirectsAreNeverFollowed(t *testing.T) {
	leaked := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("X-Hasura-Admin-Secret") != ""
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/v1/metadata", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	base, _ := Base(srv.URL)
	if _, err := Export(context.Background(), base, secret); err == nil || !strings.Contains(err.Error(), "E517") {
		t.Fatalf("redirect must fail the call: %v", err)
	}
	if leaked {
		t.Fatal("admin secret reached the redirect target")
	}
}

func TestCheckAndVersionSanitizing(t *testing.T) {
	if Check([]byte(`{"sources":[{"name":"default","tables":[]}]}`)) != nil {
		t.Fatal("a default source with no tables is structurally fine")
	}
	for _, bad := range []string{`{}`, `{"sources":[{"name":"other"}]}`} {
		if Check([]byte(bad)) == nil {
			t.Errorf("Check(%s) accepted", bad)
		}
	}
	if got := cleanVersion("v2.44.0\x1b[31m<script>"); got != "v2.44.031mscript" {
		t.Errorf("cleanVersion %q", got)
	}
	if got := cleanVersion(strings.Repeat("a", 100)); len(got) != 64 {
		t.Errorf("cleanVersion length %d", len(got))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"\u001b[2Jv2.1"}`))
	}))
	defer srv.Close()
	base, _ := Base(srv.URL)
	if v, err := Version(context.Background(), base); err != nil || v != "2Jv2.1" {
		t.Fatalf("hostile version: %q %v", v, err)
	}
}
