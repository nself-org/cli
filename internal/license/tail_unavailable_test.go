package license

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestTailStream_EndpointUnavailableStops proves a reply no retry can fix ends
// the command with ErrEventsUnavailable after exactly one request, instead of
// reconnecting forever (the live ping_api answers 404 on /license/events).
func TestTailStream_EndpointUnavailableStops(t *testing.T) {
	for _, status := range []int{
		http.StatusNotFound, http.StatusGone, http.StatusNotImplemented,
		http.StatusUnauthorized, http.StatusForbidden,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var hits atomic.Int32
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				gotPath = r.URL.Path
				w.WriteHeader(status)
			}))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var buf bytes.Buffer
			err := TailStream(ctx, TailOptions{PingURL: srv.URL, Stdout: &buf, HTTPClient: srv.Client()})
			if !errors.Is(err, ErrEventsUnavailable) {
				t.Fatalf("TailStream err = %v, want ErrEventsUnavailable", err)
			}
			if n := hits.Load(); n != 1 {
				t.Errorf("requests = %d, want 1 (no reconnect)", n)
			}
			if gotPath != "/license/events" {
				t.Errorf("path = %q, want /license/events", gotPath)
			}
			if strings.Contains(buf.String(), "reconnecting") {
				t.Errorf("must not announce a reconnect, got %q", buf.String())
			}
			if !strings.Contains(err.Error(), srv.URL) {
				t.Errorf("error should name the ping URL, got %q", err)
			}
		})
	}
}

// TestTailStream_ServerErrorStillReconnects keeps the 5xx path retrying.
func TestTailStream_ServerErrorStillReconnects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var buf bytes.Buffer
	if err := TailStream(ctx, TailOptions{PingURL: srv.URL, Stdout: &buf, HTTPClient: srv.Client()}); err != nil {
		t.Fatalf("TailStream err = %v, want nil on context end", err)
	}
	if !strings.Contains(buf.String(), "reconnecting") {
		t.Errorf("expected a reconnect notice, got %q", buf.String())
	}
}
