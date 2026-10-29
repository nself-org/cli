package httptimeout

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStripURLDropsTheRequestURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	target := srv.URL + "/p?key=synthetic-secret"
	srv.Close() // connection refused
	_, err := http.Get(target)
	if err == nil || !strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatalf("precondition: net/http error should quote the URL, got %v", err)
	}
	if got := StripURL(err); strings.Contains(got.Error(), "synthetic-secret") || strings.Contains(got.Error(), "/p") {
		t.Errorf("StripURL kept the URL: %v", got)
	}
	plain := errors.New("plain")
	if StripURL(plain) != plain || StripURL(nil) != nil {
		t.Error("non-URL errors and nil must pass through")
	}
}
