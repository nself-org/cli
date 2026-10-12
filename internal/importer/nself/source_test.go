package nself

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/portable"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeBundle writes a small valid v1 bundle (fake data only) and returns its directory.
func makeBundle(t *testing.T) string {
	t.Helper()
	w, err := portable.NewWriter(filepath.Join(t.TempDir(), "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"db/data.dump": "fake dump bytes", "db/schema.sql": "-- fake schema\n", "auth/users.jsonl": `{"id":"u1","hash_algorithm":"bcrypt"}` + "\n", "hasura/metadata.json": `{"sources":[]}`} {
		if _, err = w.WriteFile(name, strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
	}
	m := portable.Manifest{Producer: portable.Producer{Tool: "nself", Version: "1.5.0", Source: "nself"}, Postgres: portable.PostgresInfo{ServerVersion: "16.4", Major: 16}, DB: portable.DB{Schemas: []string{"public"}, Tables: []portable.Table{{Schema: "public", Name: "notes", Rows: 1, Hash: "7", PK: []string{"id"}}}}, Auth: portable.Auth{Users: 1, HashAlgorithms: map[string]int{"bcrypt": 1}}}
	if _, err = w.Finish(m); err != nil {
		t.Fatal(err)
	}
	return w.Dir()
}
func codeOf(err error) string {
	var cli *errs.CLIError
	if errors.As(err, &cli) {
		return cli.Code
	}
	return ""
}
func input(dir string) importer.Input { return importer.Input{Values: map[string]string{"dir": dir}} }
func TestRegisteredAndFlags(t *testing.T) {
	found := false
	for _, s := range importer.Sources() {
		found = found || s.Name() == "nself"
	}
	if !found {
		t.Fatal("nself source not registered")
	}
	if f := (Source{}).Flags(); len(f) != 1 || f[0].Name != "dir" || f[0].EnvVarName {
		t.Fatalf("flags: %+v", f)
	}
}
func TestDetectReadsManifest(t *testing.T) {
	d, err := Source{}.Detect(context.Background(), input(makeBundle(t)))
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != "nself" || d.Version != "16.4" || d.SourceUsers != 1 || len(d.Schemas) != 1 || d.Schemas[0] != "public" || len(d.Notes) != 1 {
		t.Fatalf("detection: %+v", d)
	}
}
func TestDirectoryRequired(t *testing.T) {
	_, err := Source{}.Detect(context.Background(), importer.Input{Values: map[string]string{}})
	if codeOf(err) != "E517" {
		t.Fatalf("want E517, got %v", err)
	}
}
func TestFlippedByteIsE516BeforeStaging(t *testing.T) {
	dir := makeBundle(t)
	p := filepath.Join(dir, "db", "data.dump")
	b, _ := os.ReadFile(p)
	b[0] ^= 0xff
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(t.TempDir(), "stage")
	_, derr := Source{}.Detect(context.Background(), input(dir))
	_, xerr := Source{}.Extract(context.Background(), input(dir), stage)
	if codeOf(derr) != "E516" || codeOf(xerr) != "E516" {
		t.Fatalf("want E516 from both, got %v / %v", derr, xerr)
	}
	if _, err := os.Stat(stage); err == nil {
		t.Fatal("a stage was created for a corrupt bundle")
	}
}
func TestSchemaVersion2IsE515(t *testing.T) {
	dir := makeBundle(t)
	p := filepath.Join(dir, "manifest.json")
	b, _ := os.ReadFile(p)
	b = bytes.Replace(b, []byte(`"schema_version": "1"`), []byte(`"schema_version": "2"`), 1)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	_, derr := Source{}.Detect(context.Background(), input(dir))
	_, xerr := Source{}.Extract(context.Background(), input(dir), filepath.Join(t.TempDir(), "stage"))
	if codeOf(derr) != "E515" || codeOf(xerr) != "E515" {
		t.Fatalf("want E515 from both, got %v / %v", derr, xerr)
	}
}
func TestExtractStagesUnchanged(t *testing.T) {
	dir := makeBundle(t)
	stage := filepath.Join(t.TempDir(), "run", "stage")
	got, err := Source{}.Extract(context.Background(), input(dir), stage)
	if err != nil || got.Stage != stage {
		t.Fatalf("extract: %+v %v", got, err)
	}
	r, err := portable.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := []string{portable.ManifestName}
	for _, f := range r.Manifest().Files {
		files = append(files, f.Path)
	}
	if len(files) != 5 {
		t.Fatalf("members: %v", files)
	}
	for _, rel := range files {
		a, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		b, err := os.ReadFile(filepath.Join(stage, filepath.FromSlash(rel)))
		if err != nil || !bytes.Equal(a, b) {
			t.Fatalf("%s differs after staging: %v", rel, err)
		}
		if runtime.GOOS != "windows" {
			if fi, _ := os.Stat(filepath.Join(stage, filepath.FromSlash(rel))); fi.Mode().Perm() != 0o600 {
				t.Fatalf("%s mode %v", rel, fi.Mode().Perm())
			}
		}
	}
	if _, err = portable.Open(stage); err != nil {
		t.Fatalf("staged bundle does not open: %v", err)
	}
	if _, err = (Source{}).Extract(context.Background(), input(dir), stage); codeOf(err) != "E520" {
		t.Fatalf("staging into an occupied path must fail with E520, got %v", err)
	}
}
func TestExtractHonoursCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Source{}).Extract(ctx, input(makeBundle(t)), filepath.Join(t.TempDir(), "stage")); codeOf(err) != "E520" {
		t.Fatalf("want E520 on cancel, got %v", err)
	}
}
func TestHardLinkedMemberIsE516(t *testing.T) {
	dir := makeBundle(t)
	if err := os.Link(filepath.Join(dir, "db", "schema.sql"), filepath.Join(t.TempDir(), "alias")); err != nil {
		t.Skip("hard links unavailable: " + err.Error())
	}
	if _, err := (Source{}).Detect(context.Background(), input(dir)); codeOf(err) != "E516" {
		t.Fatalf("want E516 for a linked member, got %v", err)
	}
}
func TestCopyManifestRefusesSwaps(t *testing.T) {
	dir := makeBundle(t)
	real := filepath.Join(dir, portable.ManifestName)
	link := filepath.Join(t.TempDir(), "link.json")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable: " + err.Error())
	}
	if err := copyManifest(context.Background(), link, filepath.Join(t.TempDir(), "out.json")); err == nil {
		t.Fatal("a symlinked manifest was copied")
	}
	if err := copyManifest(context.Background(), dir, filepath.Join(t.TempDir(), "out.json")); err == nil {
		t.Fatal("a directory was copied as the manifest")
	}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := copyManifest(context.Background(), real, out); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(real)
	if b, _ := os.ReadFile(out); !bytes.Equal(a, b) {
		t.Fatal("manifest copy differs")
	}
}
func TestCopyStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	src := io.NopCloser(strings.NewReader("data"))
	if err := writeMember(ctx, src, filepath.Join(t.TempDir(), "out")); codeOf(err) != "E520" {
		t.Fatalf("want E520 on a cancelled copy, got %v", err)
	}
}

func TestCompatSQLBundleIsRefused(t *testing.T) {
	w, err := portable.NewWriter(filepath.Join(t.TempDir(), "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"db/data.dump": "d", "db/schema.sql": "s", "db/compat.sql": "SELECT 1;\n"} {
		if _, err = w.WriteFile(name, strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = w.Finish(portable.Manifest{Postgres: portable.PostgresInfo{ServerVersion: "16.4", Major: 16}, DB: portable.DB{Schemas: []string{"public"}}}); err != nil {
		t.Fatal(err)
	}
	_, derr := Source{}.Detect(context.Background(), input(w.Dir()))
	_, xerr := Source{}.Extract(context.Background(), input(w.Dir()), filepath.Join(t.TempDir(), "stage"))
	if codeOf(derr) != "E516" || codeOf(xerr) != "E516" {
		t.Fatalf("a bundle with db/compat.sql must be refused with E516: %v / %v", derr, xerr)
	}
}
func TestDetectCarriesObjectsOutOfTheReport(t *testing.T) {
	w, err := portable.NewWriter(filepath.Join(t.TempDir(), "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	member, _ := portable.StorageMember("bkt", "a/one.txt")
	f, err := w.WriteFile(member, strings.NewReader("obj"))
	if err != nil {
		t.Fatal(err)
	}
	obj, err := portable.NewObject("bkt", "a/one.txt", f)
	if err != nil {
		t.Fatal(err)
	}
	m := portable.Manifest{Postgres: portable.PostgresInfo{ServerVersion: "16.4", Major: 16}, DB: portable.DB{Schemas: []string{"public"}}, Storage: portable.Storage{Objects: []portable.Object{obj}}, SourceCounts: []portable.SourceCount{{Kind: "objects", Count: 1, Method: "independent list"}}}
	if _, err = w.Finish(m); err != nil {
		t.Fatal(err)
	}
	d, err := Source{}.Detect(context.Background(), input(w.Dir()))
	if err != nil || len(d.Objects) != 1 || d.Objects[0].Key != "a/one.txt" {
		t.Fatalf("detection objects: %+v %v", d.Objects, err)
	}
	if b, _ := json.Marshal(d); bytes.Contains(b, []byte("one.txt")) {
		t.Fatalf("object keys must not reach the report JSON: %s", b)
	}
}
