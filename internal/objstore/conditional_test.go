package objstore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPutIfAbsent(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusPreconditionFailed, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.Header.Get("If-None-Match") != "*" {
					t.Error("missing conditional PUT")
				}
				w.WriteHeader(status)
			}))
			defer srv.Close()
			err := testClient(srv).PutIfAbsent(context.Background(), "bucket", "key", strings.NewReader("new"), 3)
			var api *Error
			if status == http.StatusOK {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.As(err, &api) || api.Status != status {
				t.Fatalf("want typed HTTP %d, got %v", status, err)
			}
		})
	}
}
