package exporter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/objstore"
	"github.com/nself-org/cli/internal/portable"
)

func TestStorageContentAddressed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("unsigned request")
		}
		if r.URL.Path == "/" {
			_, _ = io.WriteString(w, `<ListAllMyBucketsResult><Buckets><Bucket><Name>fixture</Name></Bucket></Buckets></ListAllMyBucketsResult>`)
			return
		}
		_, _ = io.WriteString(w, "fake object bytes")
	}))
	defer server.Close()
	c := &objstore.Client{Endpoint: server.URL, AccessKey: "FAKE-S3-ACCESS", SecretKey: "FAKE-S3-SECRET", Region: "us-east-1"}
	names, err := listBuckets(context.Background(), c)
	if err != nil || len(names) != 1 || names[0] != "fixture" {
		t.Fatal(names, err)
	}
	p := storagePlan{client: c, buckets: map[string][]objstore.Object{"fixture": {{Key: "A.txt", Size: 17}, {Key: "a.txt", Size: 17}, {Key: "../private", Size: 17}}}}
	w, err := portable.NewWriter(filepath.Join(t.TempDir(), "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	m := portable.Manifest{}
	if err = p.write(context.Background(), w, &m); err != nil {
		t.Fatal(err)
	}
	m, err = w.Finish(m)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte("fake object bytes"))
	for _, o := range m.Storage.Objects {
		if o.SHA256 != hex.EncodeToString(h[:]) || o.Bytes != 17 || !strings.HasPrefix(o.Member, "storage/objects/") {
			t.Fatal("wrong object mapping")
		}
	}
	if len(m.Storage.Objects) != 3 || m.Storage.Objects[0].Member == m.Storage.Objects[1].Member {
		t.Fatal("keys collided")
	}
	r, err := portable.Open(w.Dir())
	if err != nil {
		t.Fatal(err)
	}
	_ = r
	p.buckets["fixture"] = []objstore.Object{{Key: "wrong-size", Size: 18}}
	w, err = portable.NewWriter(filepath.Join(t.TempDir(), "bad"))
	if err != nil {
		t.Fatal(err)
	}
	if p.write(context.Background(), w, &portable.Manifest{}) == nil {
		t.Fatal("object size change accepted")
	}
}

func TestNoStorage(t *testing.T) {
	p, note, err := listStorage(context.Background(), &config.Config{}, true)
	if err != nil || note == "" || len(p.buckets) != 0 {
		t.Fatal("skip storage", err)
	}
	t.Setenv("NSELF_V15", "1")
	t.Setenv("OBJECT_STORAGE_ENABLED", "false")
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	p, note, err = listStorage(context.Background(), &config.Config{}, false)
	if err != nil || note != "No object store enabled" || p.client != nil {
		t.Fatal("no store", err)
	}
	t.Setenv("OBJECT_STORAGE_ENABLED", "true")
	t.Setenv("SEAWEEDFS_PORT", "invalid")
	if _, _, err = listStorage(context.Background(), &config.Config{}, false); err == nil {
		t.Fatal("invalid port accepted")
	}
	t.Setenv("SEAWEEDFS_PORT", "8333")
	if _, _, err = listStorage(context.Background(), &config.Config{}, false); err == nil {
		t.Fatal("missing credentials accepted")
	}
}
