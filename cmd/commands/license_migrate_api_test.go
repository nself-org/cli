package commands

// Purpose: fetchMigrationStatus keeps the key out of the request URL (it goes
// in X-License-Key) and never lets the key or the endpoint escape through a
// transport error, which net/http otherwise builds from the full request URL.
// Inputs: a synthetic key and httptest servers. Outputs: pass/fail.
// Constraints: loopback only.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const migrateTestKey = "synthetic key&x=1#frag"

func TestFetchMigrationStatusEncodesKey(t *testing.T) {
	var gotHeader, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader, gotPath, gotQuery = r.Header.Get("X-License-Key"), r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	if _, err := fetchMigrationStatus(context.Background(), migrateTestKey, srv.URL); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/license/migration-status" || gotQuery != "" || gotHeader != migrateTestKey {
		t.Errorf("server saw path %q query %q header %q", gotPath, gotQuery, gotHeader)
	}
}

func TestFetchMigrationStatusTransportErrorOmitsKey(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // connection refused
	_, err := fetchMigrationStatus(context.Background(), migrateTestKey, url)
	if err == nil {
		t.Fatal("want a transport error")
	}
	for _, leak := range []string{migrateTestKey, "synthetic", "key=", "migration-status"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("transport error leaks %q: %v", leak, err)
		}
	}
}
