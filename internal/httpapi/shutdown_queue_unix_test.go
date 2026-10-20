//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/invoke"
)

func TestShutdownAuditFlockHolder(t *testing.T) {
	path := os.Getenv("NSELF_TEST_AUDIT_FLOCK")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	fmt.Println("ready")
	time.Sleep(30 * time.Second)
}

func TestShutdownDrainsQueuedMutationAudits(t *testing.T) {
	for _, count := range []int{3, 4} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			s := server(t)
			var log bytes.Buffer
			s.o.Log = &log
			cmd, _ := s.o.Registry.Lookup("config get")
			cmd.SideEffect = canon.SideEffectWrite
			t.Setenv("HTTPAPI_MODE", "block")
			pids := t.TempDir()
			t.Setenv("HTTPAPI_PID", pids)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls sync.WaitGroup
			calls.Add(count)
			for i := 0; i < count; i++ {
				go func() {
					defer calls.Done()
					_, _ = s.invokeTracked(ctx, "config get", invoke.Request{}, invoke.Options{Guard: s.guard, Transport: invoke.TransportHTTP, Surface: "http", Dir: s.o.ProjectDir})
				}()
			}
			done := make(chan struct{})
			go func() { calls.Wait(); close(done) }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				entries, err := os.ReadDir(pids)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) == count {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("mutation children did not start")
				}
				time.Sleep(10 * time.Millisecond)
			}
			path, err := invoke.DefaultAuditPath()
			if err != nil {
				t.Fatal(err)
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			holder := exec.Command(exe, "-test.run=^TestShutdownAuditFlockHolder$")
			holder.Env = append(os.Environ(), "HTTPAPI_STUB=0", "NSELF_TEST_AUDIT_FLOCK="+filepath.Clean(path))
			out, err := holder.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := holder.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = holder.Process.Kill(); _ = holder.Wait(); cancel(); <-done }()
			scanner := bufio.NewScanner(out)
			if !scanner.Scan() || scanner.Text() != "ready" {
				t.Fatal("external flock holder did not start")
			}
			cancel()
			s.drainInvocations(8 * time.Second)
			select {
			case <-done:
			default:
				t.Fatal("shutdown abandoned queued mutation audits")
			}
			if bytes.Contains(log.Bytes(), []byte("drain timed out")) {
				t.Fatal("shutdown drain expired")
			}
		})
	}
}
