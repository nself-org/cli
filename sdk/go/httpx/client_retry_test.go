package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// recordingServer answers 503 for the first failFirst requests and 200 after
// that, and records the body bytes of every request it sees.
type recordingServer struct {
	mu        sync.Mutex
	failFirst int
	bodies    []string
}

func (s *recordingServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.bodies = append(s.bodies, string(b))
	n := len(s.bodies)
	s.mu.Unlock()
	if n <= s.failFirst {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *recordingServer) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...)
}

// onlyReader hides every method but Read, so http.NewRequest cannot set
// GetBody and Do has to buffer the body itself.
type onlyReader struct{ r io.Reader }

func (o onlyReader) Read(p []byte) (int, error) { return o.r.Read(p) }

// TestDo_RetryResendsBody verifies D-0152: every attempt carries the full
// request body, and MaxRetries 0 sends the request exactly once.
func TestDo_RetryResendsBody(t *testing.T) {
	const payload = `{"prompt":"synthetic-test-payload"}`
	tests := []struct {
		name       string
		maxRetries int
		failFirst  int
		body       func() io.Reader
		wantCalls  int
		wantErr    bool
	}{
		{name: "rewindable body, fail then succeed", maxRetries: 2, failFirst: 1,
			body: func() io.Reader { return strings.NewReader(payload) }, wantCalls: 2},
		{name: "plain reader body, fail then succeed", maxRetries: 2, failFirst: 1,
			body: func() io.Reader { return onlyReader{strings.NewReader(payload)} }, wantCalls: 2},
		{name: "plain reader body, fail twice then succeed", maxRetries: 2, failFirst: 2,
			body: func() io.Reader { return onlyReader{strings.NewReader(payload)} }, wantCalls: 3},
		{name: "plain reader body, retries exhausted", maxRetries: 1, failFirst: 5,
			body: func() io.Reader { return onlyReader{strings.NewReader(payload)} }, wantCalls: 2, wantErr: true},
		{name: "max retries 0, failure is not retried", maxRetries: 0, failFirst: 1,
			body: func() io.Reader { return onlyReader{strings.NewReader(payload)} }, wantCalls: 1, wantErr: true},
		{name: "max retries 0, success sends body once", maxRetries: 0, failFirst: 0,
			body: func() io.Reader { return onlyReader{strings.NewReader(payload)} }, wantCalls: 1},
		{name: "negative max retries, failure is not retried", maxRetries: -3, failFirst: 1,
			body: func() io.Reader { return strings.NewReader(payload) }, wantCalls: 1, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingServer{failFirst: tc.failFirst}
			srv := httptest.NewServer(rec)
			defer srv.Close()

			c := New(ClientOptions{MaxRetries: tc.maxRetries, RetryDelay: time.Millisecond, Timeout: 5 * time.Second})
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, tc.body())
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			resp, err := c.Do(context.Background(), req)
			if tc.wantErr {
				if err == nil {
					resp.Body.Close()
					t.Fatal("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("Do: %v", err)
				}
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Errorf("status = %d, want 200", resp.StatusCode)
				}
			}

			bodies := rec.seen()
			if len(bodies) != tc.wantCalls {
				t.Fatalf("server saw %d requests, want %d", len(bodies), tc.wantCalls)
			}
			for i, b := range bodies {
				if b != payload {
					t.Errorf("attempt %d body = %q, want %q", i+1, b, payload)
				}
			}
		})
	}
}

// TestDo_GetBodyErrorStopsRetry verifies that a failing GetBody ends Do with
// an error instead of sending an empty retry.
func TestDo_GetBodyErrorStopsRetry(t *testing.T) {
	rec := &recordingServer{failFirst: 1}
	srv := httptest.NewServer(rec)
	defer srv.Close()

	c := New(ClientOptions{MaxRetries: 2, RetryDelay: time.Millisecond, Timeout: 5 * time.Second})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, strings.NewReader("x"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	errRewind := errors.New("synthetic rewind failure")
	req.GetBody = func() (io.ReadCloser, error) { return nil, errRewind }

	_, err = c.Do(context.Background(), req)
	if !errors.Is(err, errRewind) {
		t.Fatalf("err = %v, want wrapped %v", err, errRewind)
	}
	if n := len(rec.seen()); n != 1 {
		t.Errorf("server saw %d requests, want 1", n)
	}
}
