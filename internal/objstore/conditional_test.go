package objstore

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPutIfAbsent(t *testing.T) {
	for _, tls := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsigned", true: "signed"}[tls], func(t *testing.T) {
			fake := newFakeS3()
			held := false
			puts := 0
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				puts++
				if r.Method != http.MethodPut || r.Header.Get("If-None-Match") != "*" {
					t.Errorf("want conditional PUT, got %s with %q", r.Method, r.Header.Get("If-None-Match"))
				}
				if !strings.Contains(r.Header.Get("Authorization"), "SignedHeaders=host;if-none-match;") || !fake.verify(r, r.Header.Get("X-Amz-Content-Sha256")) {
					t.Error("conditional header must be covered by a valid SigV4 signature")
				}
				if held {
					w.WriteHeader(http.StatusPreconditionFailed)
					return // Empty 412 bodies must still produce a typed collision.
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != "new" || r.ContentLength != 3 {
					t.Errorf("unexpected upload: %q, length %d", body, r.ContentLength)
				}
				held = true
			})
			var srv *httptest.Server
			if tls {
				srv = httptest.NewTLSServer(handler)
			} else {
				srv = httptest.NewServer(handler)
			}
			defer srv.Close()
			c := testClient(srv)
			ctx := context.Background()
			if err := c.PutIfAbsent(ctx, "bucket", "key", io.MultiReader(strings.NewReader("new")), 3); err != nil {
				t.Fatal(err)
			}
			err := c.PutIfAbsent(ctx, "bucket", "key", strings.NewReader("new"), 3)
			var ae *Error
			if !errors.As(err, &ae) || !ae.IsPreconditionFailed() || ae.Op != "put" {
				t.Fatalf("want typed precondition failure, got %v", err)
			}
			if puts != 2 {
				t.Fatalf("412 must not retry unconditionally: %d PUTs", puts)
			}
		})
	}
}
