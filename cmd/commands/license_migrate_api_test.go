package commands

// Purpose: fetchMigrationStatus sends the key URL-encoded in the query (the
// ping_api reads it nowhere else) and never lets it escape through a
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
	var gotKey, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotPath = r.URL.Query().Get("key"), r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	if _, err := fetchMigrationStatus(context.Background(), migrateTestKey, srv.URL); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/license/migration-status" || gotKey != migrateTestKey {
		t.Errorf("server saw path %q key %q", gotPath, gotKey)
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
