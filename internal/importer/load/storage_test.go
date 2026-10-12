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

func TestObjectsAbsentRefusesExistingKeys(t *testing.T) {
	held := map[string]bool{"/bucket/held": true}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if held[r.URL.Path] {
			_, _ = w.Write([]byte("existing"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<Error><Code>NoSuchKey</Code></Error>`))
	}))
	defer srv.Close()
	store := &objstore.Client{Endpoint: srv.URL, AccessKey: "fixture-access", SecretKey: "fixture-secret", Region: "us-east-1"}
	obj := func(key string) portable.Object { return portable.Object{Bucket: "bucket", Key: key} }
	ctx := context.Background()
	free := portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{obj("new")}}}
	if err := ObjectsAbsent(ctx, free, store, false); err != nil {
		t.Fatalf("absent keys refused: %v", err)
	}
	clash := portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{obj("new"), obj("held")}}}
	err := ObjectsAbsent(ctx, clash, store, false)
	if err == nil || !strings.Contains(err.Error(), "E519") || strings.Contains(err.Error(), "bucket/held") {
		t.Fatalf("existing key must be refused with E519 and no key name: %v", err)
	}
	if ObjectsAbsent(ctx, clash, store, true) != nil || ObjectsAbsent(ctx, clash, nil, false) != nil {
		t.Fatal("skip-storage and no store must not check")
	}
}

func TestObjectsAbsentFailsClosedOnLookupErrors(t *testing.T) {
	obj := portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{{Bucket: "bucket", Key: "k"}}}}
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		store := &objstore.Client{Endpoint: srv.URL, AccessKey: "fixture-access", SecretKey: "fixture-secret", Region: "us-east-1"}
		err := ObjectsAbsent(context.Background(), obj, store, false)
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "E521") {
			t.Errorf("status %d must refuse with E521, got %v", status, err)
		}
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	endpoint := dead.URL
	dead.Close()
	err := ObjectsAbsent(context.Background(), obj, &objstore.Client{Endpoint: endpoint, AccessKey: "fixture-access", SecretKey: "fixture-secret"}, false)
	if err == nil || !strings.Contains(err.Error(), "E521") {
		t.Errorf("unreachable store must refuse with E521, got %v", err)
	}
	if err = ObjectsAbsent(context.Background(), portable.Manifest{}, &objstore.Client{Endpoint: endpoint}, false); err != nil {
		t.Errorf("a bundle with no objects needs no store: %v", err)
	}
}

func TestApplyStorageDoesNotOverwriteAKeyThatAppearedMeanwhile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "stage")
	writer, err := portable.NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	payload := "bundle-object"
	sum := sha256.Sum256([]byte(payload))
	member, _ := portable.StorageMember("bucket", "key")
	if _, err = writer.WriteFile(member, strings.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	m, err := writer.Finish(portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{{Bucket: "bucket", Key: "key", Member: member, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(payload))}}}, SourceCounts: []portable.SourceCount{{Kind: "objects", Count: 1, Method: "independent list"}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := portable.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	put := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPut && req.URL.Path == "/bucket/key":
			put = true
		case req.Method == http.MethodGet && req.URL.Path == "/bucket/key":
			_, _ = w.Write([]byte("someone else's object"))
		}
	}))
	defer srv.Close()
	store := &objstore.Client{Endpoint: srv.URL, AccessKey: "fixture-access", SecretKey: "fixture-secret", Region: "us-east-1"}
	_, err = ApplyStorage(context.Background(), r, store, false)
	if err == nil || !strings.Contains(err.Error(), "E519") || put {
		t.Fatalf("a key that appeared after the preflight must stop the write (put=%v): %v", put, err)
	}
	_ = m
}
