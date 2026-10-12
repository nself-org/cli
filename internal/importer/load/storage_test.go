package load

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/nself-org/cli/internal/importer/verify"
	"github.com/nself-org/cli/internal/objstore"
	"github.com/nself-org/cli/internal/portable"
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

func TestKeysFreeRefusesExistingKey(t *testing.T) {
	var buckets = map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := buckets[strings.Trim(strings.SplitN(r.URL.Path, "/", 3)[1], "/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`<Error><Code>NoSuchBucket</Code></Error>`))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	store := &objstore.Client{Endpoint: srv.URL, AccessKey: "fixtureaccess", SecretKey: "fixturesecretvalue"}
	m := portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{{Bucket: "avatars", Key: "a.png"}}}}
	ctx := context.Background()
	if err := KeysFree(ctx, m, store, false); err != nil {
		t.Fatalf("missing bucket must be free: %v", err)
	}
	buckets["avatars"] = `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>other.png</Key><Size>1</Size></Contents></ListBucketResult>`
	if err := KeysFree(ctx, m, store, false); err != nil {
		t.Fatalf("other keys must be free: %v", err)
	}
	buckets["avatars"] = `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>a.png</Key><Size>1</Size></Contents></ListBucketResult>`
	err := KeysFree(ctx, m, store, false)
	if err == nil || !strings.Contains(err.Error(), "E519") || strings.Contains(err.Error(), "a.png") {
		t.Fatalf("existing key must be refused without naming the key: %v", err)
	}
	if KeysFree(ctx, m, store, true) != nil || KeysFree(ctx, m, nil, false) != nil {
		t.Fatal("skip and no store must pass")
	}
}
