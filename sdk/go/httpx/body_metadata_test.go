package httpx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type metadataTransport func(*http.Request) (*http.Response, error)

func (f metadataTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBufferedBodyMetadata(t *testing.T) {
	for _, tc := range []struct {
		name     string
		length   int64
		trailer  bool
		wantErr  bool
		inferred int64
	}{
		{"short", 9, false, true, 0}, {"long", 1, false, true, 0},
		{"unknown", 0, false, false, 4}, {"known", 4, false, false, 4},
		{"trailers", -1, true, false, -1},
		{"trailers unknown", 0, true, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "http://example.test", io.NopCloser(strings.NewReader("body")))
			if err != nil {
				t.Fatal(err)
			}
			req.ContentLength = tc.length
			if tc.trailer {
				req.Trailer = http.Header{"X-Checksum": {"done"}}
			}
			calls := 0
			c := New(ClientOptions{MaxRetries: 1})
			c.HTTP.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.ContentLength != tc.inferred {
					t.Errorf("clone length = %d, want %d", r.ContentLength, tc.inferred)
				}
				return &http.Response{StatusCode: 200, Body: http.NoBody, Header: make(http.Header)}, nil
			})
			_, err = c.Do(context.Background(), req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && calls != 0 {
				t.Error("sent mismatched body")
			}
			if req.ContentLength != tc.length {
				t.Errorf("caller length changed to %d", req.ContentLength)
			}
		})
	}
}

func TestBufferedTrailersOnWire(t *testing.T) {
	for _, length := range []int64{0, -1} {
		t.Run("chunked", func(t *testing.T) {
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := attempts.Add(1)
				data, err := io.ReadAll(r.Body)
				if err != nil || string(data) != "body" {
					t.Error("body did not arrive intact")
				}
				if r.ContentLength != -1 || r.Trailer.Get("X-Checksum") != "done" {
					t.Error("chunked trailer did not arrive")
				}
				if attempt == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
				}
			}))
			defer srv.Close()
			req, err := http.NewRequest(http.MethodPost, srv.URL, io.NopCloser(strings.NewReader("body")))
			if err != nil {
				t.Fatal(err)
			}
			req.ContentLength = length
			req.Trailer = http.Header{"X-Checksum": {"done"}}
			c := New(ClientOptions{MaxRetries: 1, RetryDelay: time.Nanosecond})
			resp, err := c.Do(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if attempts.Load() != 2 {
				t.Errorf("attempts = %d", attempts.Load())
			}
			if req.ContentLength != length {
				t.Error("caller content length changed")
			}
		})
	}
}
