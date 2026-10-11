package httpapi

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
)

func TestGracefulShutdown(t *testing.T) {
	s := server(t)
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	s.o.Invoker = func(ctx context.Context, _ *cmdregistry.Registry, _ string, _ invoke.Request, _ invoke.Options) (invoke.Result, error) {
		started <- ctx
		select {
		case <-release:
			return invoke.Result{Stdout: []byte(doc)}, nil
		case <-ctx.Done():
			return invoke.Result{}, ctx.Err()
		}
	}
	addr, cancel, stopped := startShutdownServer(t, s)
	response := shutdownRequest(addr)
	requestCtx := waitRequestStart(t, started)
	cancel()
	waitListenerClosed(t, addr)
	if err := requestCtx.Err(); err != nil {
		t.Fatalf("in-flight request canceled before graceful drain: %v", err)
	}
	close(release)
	select {
	case result := <-response:
		if result.err != nil || result.status != http.StatusOK || result.body != doc {
			t.Fatalf("drained response: %+v", result)
		}
		noCORS(t, result.header)
	case <-time.After(3 * time.Second):
		t.Fatal("released request did not finish during drain")
	}
	waitShutdown(t, stopped, 3*time.Second)
}

func TestShutdownDeadlineClosesRequests(t *testing.T) {
	s := server(t)
	started := make(chan context.Context, 1)
	finished := make(chan struct{})
	s.o.Invoker = func(ctx context.Context, _ *cmdregistry.Registry, _ string, _ invoke.Request, _ invoke.Options) (invoke.Result, error) {
		started <- ctx
		<-ctx.Done()
		close(finished)
		return invoke.Result{}, ctx.Err()
	}
	addr, cancel, stopped := startShutdownServer(t, s)
	response := shutdownRequest(addr)
	requestCtx := waitRequestStart(t, started)
	begin := time.Now()
	cancel()
	waitListenerClosed(t, addr)
	if err := requestCtx.Err(); err != nil {
		t.Fatalf("in-flight request canceled before shutdown deadline: %v", err)
	}
	waitShutdown(t, stopped, 7*time.Second)
	if elapsed := time.Since(begin); elapsed < 4500*time.Millisecond || elapsed > 7*time.Second {
		t.Fatalf("shutdown took %s; wanted five seconds plus margin", elapsed)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("forced close did not cancel remaining invocation")
	}
	select {
	case <-response:
	case <-time.After(time.Second):
		t.Fatal("client remained blocked after forced close")
	}
}

func startShutdownServer(t *testing.T, s *Server) (string, context.CancelFunc, <-chan error) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stopped := make(chan error, 1)
	go func() { stopped <- s.ListenAndServe(ctx, addr) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return addr, cancel, stopped
		}
		select {
		case err := <-stopped:
			t.Fatalf("server startup failed: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("listener did not become ready: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type shutdownResponse struct {
	status int
	body   string
	header http.Header
	err    error
}

func shutdownRequest(addr string) <-chan shutdownResponse {
	done := make(chan shutdownResponse, 1)
	go func() {
		req, err := http.NewRequest("POST", "http://"+addr+"/v1/commands/config/get", strings.NewReader("{}"))
		if err != nil {
			done <- shutdownResponse{err: err}
			return
		}
		req.Host = "localhost:3857"
		req.Header.Set("Authorization", "Bearer fake-token-0000")
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 10 * time.Second}
		defer client.CloseIdleConnections()
		res, err := client.Do(req)
		if err != nil {
			done <- shutdownResponse{err: err}
			return
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		done <- shutdownResponse{status: res.StatusCode, body: string(body), header: res.Header, err: err}
	}()
	return done
}

func waitRequestStart(t *testing.T, started <-chan context.Context) context.Context {
	t.Helper()
	select {
	case ctx := <-started:
		return ctx
	case <-time.After(3 * time.Second):
		t.Fatal("blocking invocation did not start")
		return nil
	}
}

func waitListenerClosed(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		if time.Now().After(deadline) {
			t.Fatal("shutdown did not stop accepting connections")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitShutdown(t *testing.T, stopped <-chan error, timeout time.Duration) {
	t.Helper()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(fmt.Errorf("shutdown: %w", err))
		}
	case <-time.After(timeout):
		t.Fatal("ListenAndServe did not return within shutdown window")
	}
}
