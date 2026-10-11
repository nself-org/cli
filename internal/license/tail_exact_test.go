package license

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// rtFunc adapts a function to http.RoundTripper.
type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func sseResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

const tailEventLine = `data: {"ts":"2026-10-11T10:20:30Z","key_prefix":"nself_pro_ab","plugin":"ai","result":"allow"}` + "\n"

// tailEventOut is the exact line printEvent writes for tailEventLine to a non-terminal writer.
var tailEventOut = "10:20:30  ALLOW       key=nself_pro_ab  plugin=ai" + strings.Repeat(" ", 18) + "\n"

// TestTailStreamUsesInjectedClient pins that a supplied HTTPClient is the one used and its events are printed (D-0226).
func TestTailStreamUsesInjectedClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	calls := 0
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return sseResponse(tailEventLine), nil
		}
		cancel() // second connection: stop the tail deterministically
		return nil, context.Canceled
	})}
	var out bytes.Buffer
	err := TailStream(ctx, TailOptions{PingURL: "http://tail.invalid", Stdout: &out, HTTPClient: client})
	if err != nil {
		t.Fatalf("TailStream: %v", err)
	}
	want := "Streaming license validation events (Ctrl-C to stop)...\n" + tailEventOut
	if out.String() != want {
		t.Errorf("output %q, want %q", out.String(), want)
	}
	if calls != 2 {
		t.Errorf("transport called %d times, want 2", calls)
	}
}

// TestStreamOnceRequestURL pins the request URL: no query at all without filters, ?k=v with one (D-0226).
func TestStreamOnceRequestURL(t *testing.T) {
	var got []string
	var hdr http.Header
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		got = append(got, r.URL.String())
		hdr = r.Header
		return sseResponse(""), nil
	})}
	for _, f := range []map[string]string{nil, {}, {"result": "deny"}} {
		if err := streamOnce(context.Background(), client, "http://tail.invalid", f, io.Discard); err != nil {
			t.Fatalf("streamOnce: %v", err)
		}
	}
	want := []string{"http://tail.invalid/license/events", "http://tail.invalid/license/events", "http://tail.invalid/license/events?result=deny"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request %d URL %q, want %q", i, got[i], want[i])
		}
	}
	if hdr.Get("Accept") != "text/event-stream" || hdr.Get("Cache-Control") != "no-cache" {
		t.Errorf("headers %v", hdr)
	}
	// Two filters: both appear, joined by & (map order is random).
	client = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		got = append(got, r.URL.RawQuery)
		return sseResponse(""), nil
	})}
	got = nil
	if err := streamOnce(context.Background(), client, "http://tail.invalid", map[string]string{"result": "deny", "plugin": "ai"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	q, err := url.ParseQuery(got[0])
	if err != nil || len(q) != 2 || q.Get("result") != "deny" || q.Get("plugin") != "ai" || strings.Count(got[0], "&") != 1 {
		t.Errorf("two-filter query %q", got[0])
	}
}

// TestStreamOnceNon200 pins the non-200 error text (D-0226).
func TestStreamOnceNon200(t *testing.T) {
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		resp := sseResponse("")
		resp.StatusCode = 503
		return resp, nil
	})}
	err := streamOnce(context.Background(), client, "http://tail.invalid", nil, io.Discard)
	if err == nil || err.Error() != "stream endpoint returned HTTP 503" {
		t.Errorf("got %v", err)
	}
}

// TestStreamOnceScannerError pins that an over-long line is an error while the context lives and a clean nil once it is cancelled (D-0226).
func TestStreamOnceScannerError(t *testing.T) {
	tooLong := "data: " + strings.Repeat("x", 70*1024) + "\n"

	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) { return sseResponse(tooLong), nil })}
	var out bytes.Buffer
	if err := streamOnce(context.Background(), client, "http://tail.invalid", nil, &out); !errors.Is(err, bufio.ErrTooLong) {
		t.Errorf("live ctx: got %v, want bufio.ErrTooLong", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		cancel() // the context ends before the read fails
		return sseResponse(tooLong), nil
	})}
	if err := streamOnce(ctx, client, "http://tail.invalid", nil, &out); err != nil {
		t.Errorf("cancelled ctx: got %v, want nil", err)
	}
	if out.Len() != 0 {
		t.Errorf("wrote %q, want nothing", out.String())
	}
}

// TestStreamOnceCancelledPrintsNothing pins that events arriving after the context ended are dropped, and that live ones print (D-0226).
func TestStreamOnceCancelledPrintsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		cancel()
		return sseResponse(tailEventLine), nil
	})}
	var out bytes.Buffer
	if err := streamOnce(ctx, client, "http://tail.invalid", nil, &out); err != nil || out.Len() != 0 {
		t.Errorf("cancelled: err=%v out=%q", err, out.String())
	}
	client = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return sseResponse(tailEventLine + "data: [DONE]\n: comment\ndata: not-json\n"), nil
	})}
	out.Reset()
	if err := streamOnce(context.Background(), client, "http://tail.invalid", nil, &out); err != nil || out.String() != tailEventOut {
		t.Errorf("live: err=%v out=%q, want %q", err, out.String(), tailEventOut)
	}
}

// TestNextBackoffExact pins the doubling and the 30 s cap (D-0226).
func TestNextBackoffExact(t *testing.T) {
	s := time.Second
	cases := map[time.Duration]time.Duration{
		s: 2 * s, 2 * s: 4 * s, 8 * s: 16 * s, 14 * s: 28 * s, 15 * s: 30 * s, 16 * s: 30 * s, 30 * s: 30 * s, 60 * s: 30 * s,
	}
	for in, want := range cases {
		if got := nextBackoff(in); got != want {
			t.Errorf("nextBackoff(%v) = %v, want %v", in, got, want)
		}
	}
}
