package load

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/objstore"
	"github.com/nself-org/cli/internal/portable"
)

func TestApplyStorageRefusesConcurrentObject(t *testing.T) {
	r := conditionalStorageBundle(t)
	const other = "concurrent writer's object"
	held := ""
	puts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`)
		case http.MethodHead:
			// The competing write completes after preflight, before the upload.
			held = other
		case http.MethodPut:
			puts++
			if held != "" && req.Header.Get("If-None-Match") == "*" {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			body, _ := io.ReadAll(req.Body)
			held = string(body)
		default:
			t.Errorf("unexpected method: %s", req.Method)
		}
	}))
	defer srv.Close()
	store := &objstore.Client{Endpoint: srv.URL, AccessKey: "test-access", SecretKey: "test-secret"}
	ctx := context.Background()
	if err := StorageCollisions(ctx, r.Manifest(), store, false); err != nil {
		t.Fatal(err)
	}
	_, err := ApplyStorage(ctx, r, store, false)
	if held != other {
		t.Errorf("concurrent object overwritten: %q", held)
	}
	if codeOf(err) != "E519" || strings.Contains(err.Error(), "private-key") {
		t.Errorf("want E519 without object key, got %v", err)
	}
	if puts != 1 {
		t.Errorf("want one conditional attempt, got %d", puts)
	}
}

func conditionalStorageBundle(t *testing.T) *portable.Reader {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "stage")
	w, err := portable.NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	member, err := portable.StorageMember("bucket", "private-key")
	if err != nil {
		t.Fatal(err)
	}
	f, err := w.WriteFile(member, strings.NewReader("new"))
	if err != nil {
		t.Fatal(err)
	}
	m := portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{{Bucket: "bucket", Key: "private-key", Member: member, SHA256: f.SHA256, Bytes: f.Bytes}}}}
	if _, err = w.Finish(m); err != nil {
		t.Fatal(err)
	}
	r, err := portable.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
