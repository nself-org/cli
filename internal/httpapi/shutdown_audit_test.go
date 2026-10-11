package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
)

func TestShutdownDrainsInvocationsAndAudit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM-ignoring child requires Unix")
	}
	for _, destructive := range []bool{false, true} {
		name := "write"
		if destructive {
			name = "confirmed-destructive"
		}
		t.Run(name, func(t *testing.T) { shutdownAuditedRun(t, destructive) })
	}
}

func shutdownAuditedRun(t *testing.T, destructive bool) {
	t.Helper()
	s := server(t) // Uses the real invoke.Invoke, isolated HOME and USERPROFILE.
	path, route, body := "config get", "/v1/commands/config/get", []byte("{}")
	if destructive {
		path, route = "destroy", "/v1/commands/destroy"
		confirmation := call(t, s, route, "{}", nil)
		expect(t, confirmation, http.StatusPreconditionRequired, "")
		var envelope struct {
			Data invoke.Confirmation `json:"data"`
		}
		if err := json.Unmarshal(confirmation.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Data.Confirm == "" {
			t.Fatal("missing confirmation nonce")
		}
		body, _ = json.Marshal(invoke.Request{Confirm: envelope.Data.Confirm})
	} else {
		cmd, _ := s.o.Registry.Lookup(path)
		cmd.SideEffect = canon.SideEffectWrite
	}
	cmd, _ := s.o.Registry.Lookup(path)
	var request invoke.Request
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	id := invoke.RequestID(path, cmd, request)
	t.Setenv("HTTPAPI_MODE", "shutdown")
	pidfile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("HTTPAPI_PID", pidfile)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- s.Serve(ctx, ln) }()
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		req, _ := http.NewRequest("POST", "http://"+ln.Addr().String()+route, bytes.NewReader(body))
		req.Host = "localhost:3857"
		req.Header.Set("Authorization", "Bearer fake-token-0000")
		req.Header.Set("Content-Type", "application/json")
		res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err == nil {
			_ = res.Body.Close()
		}
	}()
	pid := shutdownChildPID(t, pidfile)
	child, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Kill() }()
	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(18 * time.Second):
		t.Fatal("Serve did not finish shutdown and audit drain")
	}
	if child.Signal(syscall.Signal(0)) == nil {
		t.Fatalf("Serve returned with child %d alive", pid)
	}
	auditPath, err := invoke.DefaultAuditPath()
	if err != nil {
		t.Fatal(err)
	}
	audit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range bytes.Split(bytes.TrimSpace(audit), []byte("\n")) {
		var rec struct {
			Command   string `json:"command"`
			Decision  string `json:"decision"`
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatal(err)
		}
		found = found || (rec.Command == path && rec.Decision == invoke.DecisionRun && rec.RequestID == id)
	}
	if !found {
		t.Fatalf("Serve returned before run audit %s: %s", id, audit)
	}
	<-clientDone
}

func shutdownChildPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if data, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(string(data)); err == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("SIGTERM-ignoring child did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestShutdownClosesInvocationAdmission(t *testing.T) {
	s := server(t)
	s.drainInvocations(time.Second)
	_, err := s.invokeTracked(context.Background(), "config get", invoke.Request{}, invoke.Options{})
	if err != context.Canceled {
		t.Fatalf("post-drain invocation error = %v", err)
	}
}

func TestShutdownDrainCapLogsOnce(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "document"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) { shutdownDrainCap(t, stream) })
	}
}

func shutdownDrainCap(t *testing.T, stream bool) {
	t.Helper()
	s := server(t)
	var log bytes.Buffer
	s.o.Log = &log
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	s.o.Invoker = func(context.Context, *cmdregistry.Registry, string, invoke.Request, invoke.Options) (invoke.Result, error) {
		close(started)
		<-release
		return invoke.Result{}, nil
	}
	go func() {
		defer close(finished)
		if stream {
			s.stream(httptest.NewRecorder(), httptest.NewRequest("POST", "/", nil), "stream", invoke.Request{}, invoke.Options{})
		} else {
			_, _ = s.invokeTracked(context.Background(), "config get", invoke.Request{}, invoke.Options{})
		}
	}()
	<-started
	s.drainInvocations(time.Millisecond)
	close(release)
	<-finished
	if got := log.String(); got != "nself: HTTP shutdown invocation drain timed out\n" {
		t.Fatalf("drain expiry log = %q", got)
	}
}
