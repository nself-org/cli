package objstore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListBucketsRejectsInvalidResponses(t *testing.T) {
	for _, body := range []string{
		`<ListAllMyBucketsResult><Buckets><Bucket><Name>../escape</Name></Bucket></Buckets></ListAllMyBucketsResult>`,
		`<ListBucketResult/>`,
		`<ListAllMyBucketsResult><Buckets>`,
	} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, body)
			}))
			defer srv.Close()
			c := &Client{Endpoint: srv.URL, AccessKey: "key", SecretKey: "secret"}
			if _, err := c.ListBuckets(context.Background()); err == nil {
				t.Fatal("expected invalid bucket response to fail")
			}
		})
	}
}

func TestListBucketsRedactsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `<Error><Code>AccessDenied</Code><Message>secret credential</Message></Error>`)
	}))
	defer srv.Close()
	c := &Client{Endpoint: srv.URL, AccessKey: "key", SecretKey: "secret"}
	_, err := c.ListBuckets(context.Background())
	if err == nil || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("expected redacted S3 error, got %v", err)
	}
}
