package vectorsearch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// hasuraExporting serves one export_metadata document and returns a Config
// that points at it.
func hasuraExporting(t *testing.T, doc string) *config.Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(doc))
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := &config.Config{}
	cfg.Hasura.Port = port
	return cfg
}

func TestMetadataDriftLayouts(t *testing.T) {
	live := `{"version":3,"sources":[]}`
	cfg := hasuraExporting(t, live)

	// legacy metadata.json: equal, then a different document, then missing
	dir := t.TempDir()
	file := filepath.Join(dir, "hasura", "metadata.json")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`{"sources":[],"version":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := MetadataDrift(t.Context(), cfg, dir); err != nil || len(got) != 0 {
		t.Fatalf("equal metadata.json: drift=%v err=%v", got, err)
	}
	if err := os.WriteFile(file, []byte(`{"version":3,"sources":[{"name":"default"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := MetadataDrift(t.Context(), cfg, dir); err != nil || len(got) == 0 {
		t.Fatalf("different metadata.json: drift=%v err=%v", got, err)
	}
	if _, err := MetadataDrift(t.Context(), cfg, t.TempDir()); err == nil {
		t.Error("no on-disk metadata must be an error")
	}

	// YAML directory: goes through HasuraDiffMetadata (flat key files)
	ydir := t.TempDir()
	mdir := filepath.Join(ydir, "hasura", "metadata")
	if err := os.MkdirAll(mdir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"version.yaml": "3\n", "sources.yaml": "[]\n"} {
		if err := os.WriteFile(filepath.Join(mdir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := MetadataDrift(t.Context(), cfg, ydir); err != nil || len(got) != 0 {
		t.Fatalf("matching yaml dir: drift=%v err=%v", got, err)
	}
	if err := os.WriteFile(filepath.Join(mdir, "sources.yaml"), []byte("- name: other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := MetadataDrift(t.Context(), cfg, ydir); err != nil || len(got) == 0 {
		t.Fatalf("drifted yaml dir: drift=%v err=%v", got, err)
	}

	// live metadata unreachable
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	bad := &config.Config{}
	bad.Hasura.Port = port
	if _, err := MetadataDrift(t.Context(), bad, dir); err == nil {
		t.Error("a failing Hasura export must be an error")
	}
}

func TestQueryBuildersRejectBadIdentifiers(t *testing.T) {
	for _, mut := range []func(*Spec){
		func(s *Spec) { s.Schema = "a b" },
		func(s *Spec) { s.Table = "a b" },
		func(s *Spec) { s.Column = "a b" },
	} {
		s := docsSpec(Cosine)
		mut(&s)
		if _, err := typeQuery(s); err == nil {
			t.Errorf("typeQuery accepted %+v", s)
		}
		if err := CheckFunctionShape(t.Context(), fakeExec("t", nil, nil), PG{}, s); err == nil {
			t.Errorf("CheckFunctionShape accepted %+v", s)
		}
	}
	// the exec failure path of the shape check carries stderr
	failing := func(_ context.Context, _ string, _ []string) (string, string, error) {
		return "", "no such container", errors.New("exit 1")
	}
	err := CheckFunctionShape(t.Context(), failing, PG{Container: "c"}, docsSpec(Cosine))
	if err == nil || !strings.Contains(err.Error(), "no such container") {
		t.Errorf("err = %v", err)
	}
}

func TestMirrorEditFailures(t *testing.T) {
	if _, err := editFile(filepath.Join(t.TempDir(), "absent.json"), unmarshalJSON, marshalJSON, "public", "f", []string{"user"}); err == nil {
		t.Error("a missing file must fail")
	}
	// encode failure
	_, file := writeFixture(t, "json")
	failEnc := func(any) ([]byte, error) { return nil, errors.New("encode boom") }
	if _, err := editFile(file, unmarshalJSON, failEnc, "public", "f", []string{"user"}); err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Errorf("encode failure lost: %v", err)
	}
	// a marshal that cannot encode
	if _, err := marshalJSON(make(chan int)); err == nil {
		t.Error("marshalJSON accepted a channel")
	}
	// no default source: other source first, a non-map entry, then nothing
	dir := t.TempDir()
	ypath := filepath.Join(dir, "hasura", "metadata", "sources.yaml")
	if err := os.MkdirAll(filepath.Dir(ypath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ypath, []byte("- name: other\n- just a string\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := MirrorFunction(dir, "public", "f", []string{"user"}); !errors.Is(err, ErrUnsupportedLayout) {
		t.Errorf("no default source: err = %v", err)
	}
	// a scalar document has no sources at all
	if err := os.WriteFile(ypath, []byte("just text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := MirrorFunction(dir, "public", "f", []string{"user"}); !errors.Is(err, ErrUnsupportedLayout) {
		t.Errorf("scalar document: err = %v", err)
	}
	// a functions list holding a non-map item is skipped, the entry is still added
	body := "- name: default\n  functions:\n    - not a map\n    - function: {name: other, schema: public}\n"
	if err := os.WriteFile(ypath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := MirrorFunction(dir, "public", "f", []string{"user"}); err != nil || !changed {
		t.Errorf("odd functions list: changed=%v err=%v", changed, err)
	}
	// invalid documents
	if err := os.WriteFile(ypath, []byte("a: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := MirrorFunction(dir, "public", "f", []string{"user"}); err == nil {
		t.Error("invalid YAML must fail")
	}
}

func TestMirrorWriteFailureAndMigrationCreateFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions do not stop root")
	}
	dir, file := writeFixture(t, "json")
	if err := os.Chmod(file, 0o400); err != nil {
		t.Fatal(err)
	}
	if _, _, err := MirrorFunction(dir, "public", "docs_nearest_embedding", []string{"user"}); err == nil {
		t.Error("a read-only metadata file must fail the mirror")
	}
	if runtime.GOOS == "windows" {
		return // a read-only directory mode does not stop file creation on Windows
	}
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
	_, written, err := writeMigration(ro, docsSpec(Cosine), "SELECT 1;\n", time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	if err == nil || written || strings.Contains(err.Error(), "already exists") {
		t.Errorf("read-only dir: err=%v written=%v", err, written)
	}
}

func TestRunRejectsRenderFailure(t *testing.T) {
	// Render refuses a spec without a dimension; Run never reaches that with a
	// real inspection, so cover Render's own guard here.
	s := docsSpec(Cosine)
	s.Dim = 0
	if _, err := Render(s); err == nil || !strings.Contains(err.Error(), "dimension") {
		t.Errorf("err = %v", err)
	}
	if _, err := Render(Spec{Schema: "a b"}); err == nil {
		t.Error("Render accepted an invalid spec")
	}
}
