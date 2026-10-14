package httpapi

// Purpose: shared fixtures for the adversarial HTTP suite (P7-SURF-21).
// Inputs: the package fixtures server/call/expect from server_test.go.
// Outputs: a recording invoker, a locked log buffer and a raw-TCP test server.
// Constraints: loopback only; no sleeps; the invoker is a stub, so no child runs
//   unless a test installs the real invoke.Invoke on purpose.

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
)

// advToken is the synthetic bearer token that server(t) configures.
const advToken = "fake-token-0000"

// advInvoker records every command path that reaches the invoker. A non-nil
// gate holds each call until it is closed or the request context ends.
type advInvoker struct {
	mu      sync.Mutex
	paths   []string
	err     error
	gate    chan struct{}
	entered chan struct{}
}

// Invoke implements Invoker.
func (a *advInvoker) Invoke(ctx context.Context, _ *cmdregistry.Registry, path string, _ invoke.Request, _ invoke.Options) (invoke.Result, error) {
	a.mu.Lock()
	a.paths = append(a.paths, path)
	gate, entered, err := a.gate, a.entered, a.err
	a.mu.Unlock()
	if entered != nil {
		entered <- struct{}{}
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return invoke.Result{}, ctx.Err()
		}
	}
	if err != nil {
		return invoke.Result{}, err
	}
	return invoke.Result{Stdout: []byte(doc), RequestID: strings.Repeat("0", 64)}, nil
}

// count is the number of invocations so far.
func (a *advInvoker) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.paths)
}

// seen is a copy of the invoked command paths.
func (a *advInvoker) seen() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.paths...)
}

// lockedBuf is a goroutine-safe log sink.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// newAdv builds a server whose invoker records instead of running a child and
// whose log goes to a buffer. mutate may adjust the options before New.
func newAdv(t *testing.T, mutate func(*Options)) (*Server, *advInvoker, *lockedBuf) {
	t.Helper()
	o := server(t).o
	inv, lg := &advInvoker{}, &lockedBuf{}
	o.Invoker, o.Log = inv.Invoke, lg
	if mutate != nil {
		mutate(&o)
	}
	s, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return s, inv, lg
}

// rawSrv is a real listener serving a recording server, driven by raw bytes.
type rawSrv struct {
	s    *Server
	inv  *advInvoker
	log  *lockedBuf
	addr string
	port int
}

// startRaw serves on 127.0.0.1:0 with the listener's port as the Host port.
// Cleanup cancels the server and waits for Serve to return.
func startRaw(t *testing.T, mutate func(*Options)) *rawSrv {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	s, inv, lg := newAdv(t, func(o *Options) {
		o.Port = port
		if mutate != nil {
			mutate(o)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			t.Error("raw server did not stop")
		}
	})
	return &rawSrv{s: s, inv: inv, log: lg, addr: ln.Addr().String(), port: port}
}

// dial opens a connection with a safety deadline (a failure guard, not a wait).
func (r *rawSrv) dial(t *testing.T) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", r.addr)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// req renders one POST request. Header lines in extra replace nothing: the
// caller passes every header it wants besides Host and the default auth pair.
func (r *rawSrv) req(target, body string, extra ...string) string {
	h := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: localhost:%d\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\n", target, r.port, advToken)
	for _, e := range extra {
		h += e + "\r\n"
	}
	if !strings.Contains(strings.Join(extra, "\n"), "Content-Length") && !strings.Contains(strings.Join(extra, "\n"), "Transfer-Encoding") {
		h += fmt.Sprintf("Content-Length: %d\r\n", len(body))
	}
	return h + "\r\n" + body
}

// readResponses reads up to n responses from c; it stops early on EOF.
func readResponses(t *testing.T, c net.Conn, n int) []*http.Response {
	t.Helper()
	br := bufio.NewReader(c)
	var out []*http.Response
	for i := 0; i < n; i++ {
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			break
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(b))
		out = append(out, resp)
	}
	return out
}

// send writes raw on a fresh connection and returns the first response status
// (0 when the server closed without answering) and its body.
func (r *rawSrv) send(t *testing.T, raw string) (int, string) {
	t.Helper()
	c := r.dial(t)
	if _, err := io.WriteString(c, raw); err != nil {
		return 0, ""
	}
	rs := readResponses(t, c, 1)
	if len(rs) == 0 {
		return 0, ""
	}
	b, _ := io.ReadAll(rs[0].Body)
	return rs[0].StatusCode, string(b)
}

// hasCORS reports any Access-Control-* header in h.
func hasCORS(h http.Header) bool {
	for k := range h {
		if strings.HasPrefix(strings.ToLower(k), "access-control-") {
			return true
		}
	}
	return false
}
