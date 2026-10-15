package httpx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDo_BufferedBodyRejectsContentLengthMismatch(t *testing.T) {
	for _, length := range []int64{3, 100} {
		t.Run(strconv.FormatInt(length, 10), func(t *testing.T) {
			rec := &recordingServer{}
			srv := httptest.NewServer(rec)
			defer srv.Close()
			req, err := http.NewRequest(http.MethodPost, srv.URL, onlyReader{strings.NewReader("12345")})
			if err != nil {
				t.Fatal(err)
			}
			req.ContentLength = length
			c := New(ClientOptions{MaxRetries: 1, RetryDelay: time.Millisecond})
			resp, err := c.Do(context.Background(), req)
			if resp != nil {
				resp.Body.Close()
			}
			if err == nil || !strings.Contains(err.Error(), "ContentLength") {
				t.Errorf("Do error = %v, want ContentLength mismatch", err)
			}
			if calls := len(rec.seen()); calls != 0 {
				t.Errorf("sent %d requests with a mismatched body, want 0", calls)
			}
			if req.ContentLength != length {
				t.Errorf("caller ContentLength = %d, want %d", req.ContentLength, length)
			}
		})
	}
}

func TestDo_BufferedBodyPreservesMetadata(t *testing.T) {
	for _, tc := range []struct {
		name    string
		length  int64
		trailer bool
	}{
		{name: "zero length"},
		{name: "unknown length", length: -1},
		{name: "explicit length", length: 5},
		{name: "zero length with trailer", trailer: true},
		{name: "unknown length with trailer", length: -1, trailer: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type received struct {
				body, trailer string
				length        int64
			}
			requests := make(chan received, 2)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				requests <- received{string(body), r.Trailer.Get("X-Checksum"), r.ContentLength}
				if len(requests) == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
				}
			}))
			defer srv.Close()
			req, err := http.NewRequest(http.MethodPost, srv.URL, onlyReader{strings.NewReader("12345")})
			if err != nil {
				t.Fatal(err)
			}
			req.ContentLength = tc.length
			if tc.trailer {
				req.Trailer = http.Header{"X-Checksum": {"checksum"}}
			}
			c := New(ClientOptions{MaxRetries: 1, RetryDelay: time.Millisecond})
			resp, err := c.Do(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if req.ContentLength != tc.length {
				t.Errorf("caller ContentLength = %d, want %d", req.ContentLength, tc.length)
			}
			if len(requests) != 2 {
				t.Fatalf("received %d requests, want 2", len(requests))
			}
			for range 2 {
				got := <-requests
				want := received{body: "12345", length: 5}
				if tc.trailer {
					want.trailer, want.length = "checksum", -1
				}
				if got != want {
					t.Errorf("request = %+v, want %+v", got, want)
				}
			}
		})
	}
}
