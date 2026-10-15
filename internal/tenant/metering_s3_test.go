package tenant

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nself-org/cli/internal/objstore"
)

// TestBucketBytesSumsObjects: SeaweedFS metering sums listed sizes; a missing bucket is 0 (D-0683).
func TestBucketBytesSumsObjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/acme" {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `<Error><Code>NoSuchBucket</Code></Error>`)
			return
		}
		fmt.Fprint(w, `<ListBucketResult><Contents><Key>a</Key><Size>10</Size></Contents><Contents><Key>b</Key><Size>32</Size></Contents></ListBucketResult>`)
	}))
	defer srv.Close()
	c := &objstore.Client{Endpoint: srv.URL, AccessKey: "k", SecretKey: "s"}
	if got, err := bucketBytes(context.Background(), c, "acme"); err != nil || got != 42 {
		t.Fatalf("acme = %d, %v; want 42", got, err)
	}
	if got, err := bucketBytes(context.Background(), c, "gone"); err != nil || got != 0 {
		t.Fatalf("missing bucket = %d, %v; want 0", got, err)
	}
}
