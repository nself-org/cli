package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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

// TestDo_OversizedBodyIsNotRetried verifies that a body without GetBody
// larger than maxBufferedBody is sent once, whole, and not retried.
func TestDo_OversizedBodyIsNotRetried(t *testing.T) {
	old := maxBufferedBody
	maxBufferedBody = 4
	defer func() { maxBufferedBody = old }()

	rec := &recordingServer{failFirst: 1}
	srv := httptest.NewServer(rec)
	defer srv.Close()

	const payload = "longer-than-four-bytes"
	c := New(ClientOptions{MaxRetries: 2, RetryDelay: time.Millisecond, Timeout: 5 * time.Second})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL,
		onlyReader{strings.NewReader(payload)})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if _, err := c.Do(context.Background(), req); err == nil {
		t.Fatal("expected error from the single 503 attempt, got nil")
	}
	bodies := rec.seen()
	if len(bodies) != 1 || bodies[0] != payload {
		t.Fatalf("server saw %q, want one request with %q", bodies, payload)
	}
}

// TestDo_BufferedBodySendsContentLength verifies that a buffered body goes out
// with a Content-Length instead of chunked encoding.
func TestDo_BufferedBodySendsContentLength(t *testing.T) {
	var mu sync.Mutex
	var lengths []int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		lengths = append(lengths, r.ContentLength)
		n := len(lengths)
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(ClientOptions{MaxRetries: 1, RetryDelay: time.Millisecond, Timeout: 5 * time.Second})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL,
		onlyReader{strings.NewReader("12345")})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(lengths) != 2 || lengths[0] != 5 || lengths[1] != 5 {
		t.Errorf("Content-Length per attempt = %v, want [5 5]", lengths)
	}
}

// TestDo_RedirectReplaysBufferedBody verifies that a 307 redirect after a
// buffered body still carries the body to the new location.
func TestDo_RedirectReplaysBufferedBody(t *testing.T) {
	var mu sync.Mutex
	var final string
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		final = string(b)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	const payload = "redirected-body"
	c := New(ClientOptions{MaxRetries: 1, RetryDelay: time.Millisecond, Timeout: 5 * time.Second})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/start",
		onlyReader{strings.NewReader(payload)})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	mu.Lock()
	defer mu.Unlock()
	if final != payload {
		t.Errorf("body at redirect target = %q, want %q", final, payload)
	}
}

// TestDo_ContentLengthMismatchErrors verifies that an explicit ContentLength
// that differs from the buffered body is rejected, not sent truncated.
func TestDo_ContentLengthMismatchErrors(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(ClientOptions{MaxRetries: 1, RetryDelay: time.Millisecond, Timeout: 5 * time.Second})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL,
		onlyReader{strings.NewReader(strings.Repeat("x", 100))})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.ContentLength = 10
	resp, err := c.Do(context.Background(), req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("Do accepted ContentLength 10 for a 100 byte body")
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Errorf("server was contacted %d times, want 0", hits)
	}
}

// TestDo_DoesNotMutateCallerContentLength verifies the inferred length lands
// on the per-attempt clone only.
func TestDo_DoesNotMutateCallerContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(ClientOptions{MaxRetries: 1, RetryDelay: time.Millisecond, Timeout: 5 * time.Second})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL,
		onlyReader{strings.NewReader("12345")})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if req.ContentLength != 0 {
		t.Errorf("caller ContentLength = %d, want it untouched (0)", req.ContentLength)
	}
}

// TestDo_TrailerKeepsChunkedEncoding verifies that a request with Trailer set
// is not given a Content-Length, so its trailers still go out.
func TestDo_TrailerKeepsChunkedEncoding(t *testing.T) {
	var mu sync.Mutex
	var gotLen int64
	var gotTrailer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		gotLen = r.ContentLength
		gotTrailer = r.Trailer.Get("X-Sum")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(ClientOptions{MaxRetries: 1, RetryDelay: time.Millisecond, Timeout: 5 * time.Second})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL,
		onlyReader{strings.NewReader("12345")})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Trailer = http.Header{"X-Sum": {"abc"}}
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	mu.Lock()
	defer mu.Unlock()
	if gotLen != -1 || gotTrailer != "abc" {
		t.Errorf("server saw ContentLength %d trailer %q, want -1 and %q", gotLen, gotTrailer, "abc")
	}
}
