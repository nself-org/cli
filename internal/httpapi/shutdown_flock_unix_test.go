//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
)

// A separate process owns the flock until the parent closes its stdin.
func TestShutdownAuditLockHolder(t *testing.T) {
	path := os.Getenv("NSELF_TEST_AUDIT_LOCK")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	_, _ = os.Stdout.WriteString("locked\n")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}

func TestShutdownDrainsQueuedAuditsUnderExternalFlock(t *testing.T) {
	s := server(t)
	cmd, _ := s.o.Registry.Lookup("config get")
	cmd.SideEffect = canon.SideEffectWrite
	path, err := invoke.DefaultAuditPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	holder := exec.Command(exe, "-test.run=^TestShutdownAuditLockHolder$")
	holder.Env = append(os.Environ(), "HTTPAPI_STUB=0", "NSELF_TEST_AUDIT_LOCK="+path)
	input, err := holder.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close(); _ = holder.Wait() }()
	if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("holder readiness: %q %v", line, err)
	}

	ready := make(chan struct{}, 3)
	release := make(chan struct{})
	original := s.o.Invoker
	s.o.Invoker = func(ctx context.Context, reg *cmdregistry.Registry, path string, req invoke.Request, opts invoke.Options) (invoke.Result, error) {
		ready <- struct{}{}
		<-release
		return original(ctx, reg, path, req, opts)
	}
	var workers sync.WaitGroup
	for i := 0; i < 3; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, _ = s.invokeTracked(context.Background(), "config get", invoke.Request{}, invoke.Options{Transport: invoke.TransportHTTP, Guard: s.guard, Dir: s.o.ProjectDir})
		}()
	}
	for i := 0; i < 3; i++ {
		<-ready
	}
	budget := s.invocationDrainBudget()
	if budget < 21*time.Second {
		t.Fatalf("three serialized writers got only %v", budget)
	}
	close(release)
	// Two writers exhaust their own five-second flock waits. The third can
	// still append once the external owner releases after the old 11 s cap.
	timer := time.AfterFunc(12*time.Second, func() { _ = input.Close() })
	defer timer.Stop()
	s.drainInvocations(budget)
	s.invokeMu.Lock()
	remaining := s.activeInvocations
	s.invokeMu.Unlock()
	workers.Wait()
	if remaining != 0 {
		t.Fatalf("drain returned with %d audit writers outstanding", remaining)
	}
	audit, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(audit, []byte(`"decision":"run"`)) {
		t.Fatal("shutdown abandoned the queued mutation audit")
	}
}
