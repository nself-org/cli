package database

// Purpose: pins the one metadata layout shared by export, archive and apply (D-0272).
// Inputs: testdata/hasura_archive_layout/metadata.json and its golden/ tree.
// Outputs: pass/fail. Regenerate the golden tree after a deliberate layout
// change with: go test ./internal/database/ -run HasuraLayout -update-hasura-golden
// Constraints: no Hasura and no network except a loopback httptest server.

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

var updateHasuraGolden = flag.Bool("update-hasura-golden", false, "rewrite hasura_archive_layout golden files")

const layoutTestdata = "testdata/hasura_archive_layout"

func layoutInput(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(layoutTestdata, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// readTree returns file name -> bytes for the regular files directly in dir.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(b)
	}
	return out
}

func unpackLayout(t *testing.T, data []byte) string {
	t.Helper()
	var b bytes.Buffer
	if err := packMetadataJSON(data, &b); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "metadata")
	if err := Unpack(&b, dest); err != nil {
		t.Fatal(err)
	}
	return dest
}

// The archive file list and bytes match the golden tree.
func TestHasuraLayoutArchiveMatchesGolden(t *testing.T) {
	dest := unpackLayout(t, layoutInput(t))
	golden := filepath.Join(layoutTestdata, "golden")
	if *updateHasuraGolden {
		if err := os.RemoveAll(golden); err != nil {
			t.Fatal(err)
		}
		if err := writeMetadataTree(golden, layoutInput(t)); err != nil {
			t.Fatal(err)
		}
	}
	got, want := readTree(t, dest), readTree(t, golden)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("archive layout differs from %s; if the change is intended run: go test ./internal/database/ -run HasuraLayout -update-hasura-golden\ngot files %v\nwant files %v", golden, keysOf(got), keysOf(want))
	}
	if want["config.yaml"] == "" || want["version.yaml"] != "3\n" {
		t.Fatalf("golden must hold config.yaml and version.yaml (3): %q", want)
	}
}

func keysOf(m map[string]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Resolving the includes gives back the input JSON, and apply's validation accepts the tree.
func TestHasuraLayoutResolvesToInput(t *testing.T) {
	data := layoutInput(t)
	dest := unpackLayout(t, data)
	if err := validateArchiveIncludes(dest); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveIncludes(dest, filepath.Join(dest, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc interface{}
	if err := yaml.Unmarshal(resolved, &doc); err != nil {
		t.Fatalf("resolved config is not YAML: %v\n%s", err, resolved)
	}
	sameJSON(t, convertYAMLToJSON(doc), data)
}

// sameJSON compares a decoded value with raw JSON semantically.
func sameJSON(t *testing.T, got interface{}, wantJSON []byte) {
	t.Helper()
	gb, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var g, w interface{}
	if err := json.Unmarshal(gb, &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wantJSON, &w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("metadata changed in the round trip:\ngot  %s\nwant %s", gb, wantJSON)
	}
}

// The local export writes the same bytes as the archive, and the layout is deterministic.
func TestHasuraLayoutExportEqualsArchive(t *testing.T) {
	data := layoutInput(t)
	exportDir := filepath.Join(t.TempDir(), "hasura", "metadata")
	if err := writeMetadataTree(exportDir, data); err != nil {
		t.Fatal(err)
	}
	if got, want := readTree(t, exportDir), readTree(t, unpackLayout(t, data)); !reflect.DeepEqual(got, want) {
		t.Fatalf("export and archive differ:\nexport %v\narchive %v", keysOf(got), keysOf(want))
	}
	a, _ := metadataLayout(data)
	b, _ := metadataLayout(data)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("layout is not deterministic")
	}
}

// Keep-chomp YAML strings must preserve every trailing newline through includes.
func TestHasuraLayoutPreservesTrailingNewlines(t *testing.T) {
	data := []byte(`{"actions":[{"definition":{"handler":"https://example.test\n\n\n"}}]}`)
	dir := t.TempDir()
	if err := writeMetadataTree(dir, data); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveIncludes(dir, filepath.Join(dir, metadataIndexFile))
	if err != nil {
		t.Fatal(err)
	}
	var doc interface{}
	if err := yaml.Unmarshal(resolved, &doc); err != nil {
		t.Fatal(err)
	}
	sameJSON(t, convertYAMLToJSON(doc), data)
}

// An older monolithic archive (one config.yaml, everything inline) still resolves.
func TestHasuraLayoutLegacyMonolithicStillResolves(t *testing.T) {
	data := layoutInput(t)
	var v interface{}
	_ = json.Unmarshal(data, &v)
	yml, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), yml, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateArchiveIncludes(dir); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveIncludes(dir, filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc interface{}
	if err := yaml.Unmarshal(resolved, &doc); err != nil {
		t.Fatal(err)
	}
	sameJSON(t, convertYAMLToJSON(doc), data)
}

// A top-level key that is not a safe file name is rejected, and nothing is written.
func TestHasuraLayoutRejectsBadKeys(t *testing.T) {
	for _, key := range []string{"", "config", "a/b", `a\b`, "..", "x..y", ".hidden", "a b", "a:b", "a#b"} {
		raw, _ := json.Marshal(map[string]interface{}{key: 1, "version": 3})
		var b bytes.Buffer
		if err := packMetadataJSON(raw, &b); err == nil || b.Len() != 0 {
			t.Errorf("key %q: want an error and no output, got %v (%d bytes)", key, err, b.Len())
		}
		dir := filepath.Join(t.TempDir(), "m")
		if err := writeMetadataTree(dir, raw); err == nil {
			t.Errorf("key %q: writeMetadataTree must fail", key)
		} else if _, serr := os.Stat(dir); serr == nil {
			t.Errorf("key %q: no directory may be created on a bad key", key)
		}
	}
	if _, err := metadataLayout([]byte(`[1]`)); err == nil {
		t.Error("a non-object export must be rejected")
	}
}

// HasuraDiffMetadata must not report the config.yaml index as an "only on disk" key.
func TestHasuraLayoutDiffIgnoresIndexFile(t *testing.T) {
	data := layoutInput(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) }))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := &config.Config{}
	cfg.Hasura.Port = port
	project := t.TempDir()
	if err := writeMetadataTree(filepath.Join(project, "hasura", "metadata"), data); err != nil {
		t.Fatal(err)
	}
	diffs, err := HasuraDiffMetadata(context.Background(), cfg, project)
	if err != nil || len(diffs) != 0 {
		t.Fatalf("a fresh export must show no drift, got %v, %v", diffs, err)
	}
}
