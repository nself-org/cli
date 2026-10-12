package exporter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/portable"
)

func TestPgPatternQuotes(t *testing.T) {
	if got := pgPattern(`a"b`); got != `"a""b"` {
		t.Fatal("identifier not quoted", got)
	}
}

func TestFreeSpaceReadsDirectory(t *testing.T) {
	if _, err := freeSpace(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := freeSpace(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing directory accepted")
	}
}

func TestDiskCheckBranches(t *testing.T) {
	out := filepath.Join(t.TempDir(), "not", "yet", "there")
	probe := func(free uint64, err error) func(string) (uint64, error) {
		return func(string) (uint64, error) { return free, err }
	}
	if err := diskCheck(out, 10, 10, probe(1<<30, nil)); err != nil {
		t.Fatal("enough space refused", err)
	}
	assertCode(t, diskCheck(out, 10, 10, probe(5, nil)), "E525")
	assertCode(t, diskCheck(out, ^uint64(0), 1, probe(1<<30, nil)), "E525")
	assertCode(t, diskCheck(out, ^uint64(0)/6*5, 1<<62, probe(1<<30, nil)), "E525")
	if err := diskCheck(out, 1, 1, probe(0, errors.New("fake probe failure"))); err == nil {
		t.Fatal("probe failure accepted")
	}
	if err := diskCheck(out, 1, 1, nil); err != nil {
		t.Fatal("real probe refused a tiny export", err)
	}
}

func TestMetadataMalformedShapes(t *testing.T) {
	db := portable.DB{Schemas: []string{"public"}, Tables: []portable.Table{{Schema: "public", Name: "notes"}}}
	if _, _, err := filterMetadata([]byte(`{"sources":["not-an-object"]}`), db); err == nil {
		t.Fatal("malformed source accepted")
	}
	if _, _, err := filterMetadata([]byte(`{"sources":[{"name":"default","tables":["bad"]}]}`), db); err == nil {
		t.Fatal("malformed table accepted")
	}
	raw := `{"metadata":{"sources":[{"name":"default","kind":"postgres","tables":[{"x":1},{"table":{"schema":"public","name":"notes"},"object_relationships":[{}],"array_relationships":[{},{}]}],"functions":["bad",{"x":1},{"function":{"schema":"public","name":"f"}}]}]}}`
	b, c, err := filterMetadata([]byte(raw), db)
	if err != nil || len(b) == 0 {
		t.Fatal(err)
	}
	if c.Tables != 1 || c.Relationships != 3 || c.Functions != 1 {
		t.Fatalf("counts %+v", c)
	}
}

func TestListStorageThroughLoopback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = io.WriteString(w, `<ListAllMyBucketsResult><Buckets><Bucket><Name>fixture</Name></Bucket></Buckets></ListAllMyBucketsResult>`)
			return
		}
		_, _ = io.WriteString(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>one.txt</Key><Size>5</Size></Contents></ListBucketResult>`)
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_V15", "1")
	t.Setenv("OBJECT_STORAGE_ENABLED", "true")
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	t.Setenv("SEAWEEDFS_PORT", u.Port())
	cfg := &config.Config{}
	cfg.Minio.S3AccessKey, cfg.Minio.S3SecretKey = "FAKE-S3-ACCESS-0000", "FAKE-S3-SECRET-0000"
	p, note, err := listStorage(context.Background(), cfg, false)
	if err != nil || note != "" || p.bytes != 5 || len(p.buckets["fixture"]) != 1 {
		t.Fatalf("plan %+v note %q err %v", p, note, err)
	}
	t.Setenv("SEAWEEDFS_PORT", "70000")
	if _, _, err = listStorage(context.Background(), cfg, false); err == nil {
		t.Fatal("out-of-range port accepted")
	}
	server.Close()
	t.Setenv("SEAWEEDFS_PORT", u.Port())
	if _, _, err = listStorage(context.Background(), cfg, false); err == nil {
		t.Fatal("unreachable store accepted")
	}
}

func TestStartedRefusesWithoutProject(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := started(context.Background(), &config.Config{}, t.TempDir()); err == nil {
		t.Fatal("unstarted project accepted")
	}
	t.Chdir(t.TempDir())
	if err := started(context.Background(), &config.Config{}, ""); err == nil {
		t.Fatal("unstarted cwd project accepted")
	}
}
