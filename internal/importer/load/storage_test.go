package load

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/nself-org/cli/internal/importer/verify"
	"github.com/nself-org/cli/internal/objstore"
	"github.com/nself-org/cli/internal/portable"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorageMissingAndExemptions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "stage")
	writer, err := portable.NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	bytes := "fixture-object"
	hash := sha256.Sum256([]byte(bytes))
	member, err := portable.StorageMember("bucket", "nested/key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.WriteFile(member, strings.NewReader(bytes)); err != nil {
		t.Fatal(err)
	}
	m := portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{{Bucket: "bucket", Key: "nested/key", Member: member, SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(bytes))}}}, SourceCounts: []portable.SourceCount{{Kind: "objects", Count: 1, Method: "independent list"}}}
	m, err = writer.Finish(m)
	if err != nil {
		t.Fatal(err)
	}
	r, err := portable.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = StoragePreflight(m, nil, false); err == nil || !strings.Contains(err.Error(), "E529") {
		t.Fatal("missing store not refused")
	}
	exemptions, err := ApplyStorage(context.Background(), r, nil, true)
	if err != nil || len(exemptions) != 1 || exemptions[0].ID != "bucket/nested/key" {
		t.Fatalf("exemptions: %+v %v", exemptions, err)
	}
	m.Exemptions = exemptions
	diffs := verify.CompareState(verify.State{Objects: map[string]string{}, Counts: map[string]int64{"objects\x00": 0}}, m, nil, nil)
	if len(diffs) != 0 {
		t.Fatalf("skip must pass with independent counts: %+v", diffs)
	}
}

type fakeLister struct {
	held map[string][]string
	err  error
}

func (f fakeLister) List(_ context.Context, bucket, _ string) ([]objstore.Object, error) {
	if f.err != nil {
		return nil, f.err
	}
	objs := []objstore.Object{}
	for _, k := range f.held[bucket] {
		objs = append(objs, objstore.Object{Key: k})
	}
	return objs, nil
}

func TestStorageCollisions(t *testing.T) {
	m := portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{{Bucket: "b", Key: "one.txt"}, {Bucket: "b", Key: "nested/two.txt"}}}}
	ctx := context.Background()
	if err := StorageCollisions(ctx, m, fakeLister{held: map[string][]string{"b": {"other.txt", "ONE.txt"}}}, false); err != nil {
		t.Fatalf("different keys (case-sensitive) must pass: %v", err)
	}
	err := StorageCollisions(ctx, m, fakeLister{held: map[string][]string{"b": {"x", "nested/two.txt"}}}, false)
	if codeOf(err) != "E519" || strings.Contains(err.Error(), "two.txt") {
		t.Fatalf("an existing key must be refused with E519 and no key name: %v", err)
	}
	if err = StorageCollisions(ctx, m, fakeLister{err: &objstore.Error{Op: "list", Status: http.StatusNotFound, Code: "NoSuchBucket"}}, false); err != nil {
		t.Fatalf("a missing bucket is empty: %v", err)
	}
	if codeOf(StorageCollisions(ctx, m, fakeLister{err: errors.New("boom")}, false)) != "E521" {
		t.Fatal("a failed listing must stop the import before any write")
	}
	if err = StorageCollisions(ctx, m, fakeLister{err: errors.New("boom")}, true); err != nil {
		t.Fatalf("--skip-storage loads no object, so nothing can collide: %v", err)
	}
	if err = StorageCollisions(ctx, portable.Manifest{}, fakeLister{err: errors.New("boom")}, false); err != nil {
		t.Fatalf("a bundle without objects never lists: %v", err)
	}
	// SeaweedFS stores "a//one.txt" and "/a/one.txt" as "a/one.txt": they must collide with it.
	for _, key := range []string{"a//one.txt", "/a/one.txt", "a/one.txt/"} {
		bm := portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{{Bucket: "b", Key: key}}}}
		if codeOf(StorageCollisions(ctx, bm, fakeLister{held: map[string][]string{"b": {"a/one.txt"}}}, false)) != "E519" {
			t.Fatalf("key %q must collide with a/one.txt", key)
		}
		if err = StorageCollisions(ctx, bm, fakeLister{held: map[string][]string{"b": {"other"}}}, false); err != nil {
			t.Fatalf("key %q with nothing held must pass: %v", key, err)
		}
	}
	for _, key := range []string{"a/../b", "./a", "a/./b", "/", "//"} {
		bm := portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{{Bucket: "b", Key: key}}}}
		if codeOf(StorageCollisions(ctx, bm, fakeLister{}, false)) != "E519" {
			t.Fatalf("key %q has no canonical form and must be refused", key)
		}
	}
	twin := portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{{Bucket: "b", Key: "a/x"}, {Bucket: "b", Key: "a//x"}}}}
	if codeOf(StorageCollisions(ctx, twin, fakeLister{}, false)) != "E519" {
		t.Fatal("two keys that collapse to one must be refused")
	}
	other := portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{{Bucket: "b", Key: "a/x"}, {Bucket: "c", Key: "a//x"}}}}
	if err = StorageCollisions(ctx, other, fakeLister{}, false); err != nil {
		t.Fatalf("the same key in two buckets is fine: %v", err)
	}
}

// TestApplyStorageRefusesRacedKey: an object that appears after the preflight listing is refused
// by the conditional PUT and the load stops with E519 (D-0774).
func TestApplyStorageRefusesRacedKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "stage")
	writer, err := portable.NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	body := "fixture-object"
	hash := sha256.Sum256([]byte(body))
	member, err := portable.StorageMember("bucket", "raced")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.WriteFile(member, strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	m := portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{{Bucket: "bucket", Key: "raced", Member: member, SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(body))}}}, SourceCounts: []portable.SourceCount{{Kind: "objects", Count: 1, Method: "independent list"}}}
	if _, err = writer.Finish(m); err != nil {
		t.Fatal(err)
	}
	r, err := portable.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var puts, conditional int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case req.Method == http.MethodPut:
			puts++
			if req.Header.Get("If-None-Match") == "*" {
				conditional++
			}
			// Another writer created the key after the preflight.
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = io.WriteString(w, `<Error><Code>PreconditionFailed</Code><Message>exists</Message></Error>`)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()
	store := &objstore.Client{Endpoint: srv.URL, AccessKey: "AKIAFAKEFAKEFAKE0001", SecretKey: "not-a-real-secret-for-tests"}
	_, err = ApplyStorage(context.Background(), r, store, false)
	if codeOf(err) != "E519" || strings.Contains(err.Error(), "raced") {
		t.Fatalf("a key created after the preflight must give E519 without the key name: %v", err)
	}
	if puts != 1 || conditional != 1 {
		t.Fatalf("puts=%d conditional=%d: the PUT must carry If-None-Match: *", puts, conditional)
	}
}
