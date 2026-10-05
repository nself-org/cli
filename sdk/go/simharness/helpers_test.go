package simharness

// Purpose: test doubles shared by the unit tests: a fake TB, a fake docker, an in-process sshd banner server.
// Constraints: no Docker needed; the fake replaces commandContext and records every argv.

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeTB implements TB; Fatalf and Skip end the goroutine like testing does.
type fakeTB struct {
	mu       sync.Mutex
	cleanups []func()
	dir      string
	fatal    string
	skipped  bool
	keep     bool // leave the fleet running: skip the cleanups
}

func (f *fakeTB) Helper()             {}
func (f *fakeTB) Logf(string, ...any) {}
func (f *fakeTB) TempDir() string     { return f.dir }
func (f *fakeTB) Cleanup(fn func())   { f.mu.Lock(); f.cleanups = append(f.cleanups, fn); f.mu.Unlock() }
func (f *fakeTB) Fatalf(format string, a ...any) {
	f.fatal = fmt.Sprintf(format, a...)
	runtime.Goexit()
}
func (f *fakeTB) Skip(...any) { f.skipped = true; runtime.Goexit() }

// run executes fn on its own goroutine, then runs the cleanups in reverse
// order, as the testing package does after a t.Fatal.
func (f *fakeTB) run(fn func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			for i := len(f.cleanups) - 1; i >= 0 && !f.keep; i-- {
				f.cleanups[i]()
			}
		}()
		fn()
	}()
	<-done
}

func newFakeTB(t *testing.T) *fakeTB { return &fakeTB{dir: t.TempDir()} }

// fakeDocker records argv and answers like a docker daemon would.
type fakeDocker struct {
	mu      sync.Mutex
	calls   [][]string
	port    int
	nextID  int
	byFleet map[string][]string // fleet id -> container ids
	failOn  string              // "<arg0> <arg1>" prefix that exits non-zero
	missing map[string]bool     // image tags that "image inspect" does not find
}

func (d *fakeDocker) record(args []string) { d.calls = append(d.calls, append([]string(nil), args...)) }

// reply answers one call and returns the command to run in its place.
func (d *fakeDocker) reply(ctx context.Context, args []string) *exec.Cmd {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.record(args)
	if d.failOn != "" && strings.HasPrefix(strings.Join(args, " "), d.failOn) {
		return exec.CommandContext(ctx, "false")
	}
	out := ""
	switch {
	case args[0] == "run":
		d.nextID++
		out = "cid" + strconv.Itoa(d.nextID)
		for i, a := range args {
			if a == "--label" && strings.HasPrefix(args[i+1], LabelFleet+"=") {
				id := strings.TrimPrefix(args[i+1], LabelFleet+"=")
				d.byFleet[id] = append(d.byFleet[id], out)
			}
		}
	case args[0] == "port":
		out = "127.0.0.1:" + strconv.Itoa(d.port)
	case args[0] == "ps":
		for _, a := range args {
			if strings.HasPrefix(a, "label="+LabelFleet+"=") {
				out = strings.Join(d.byFleet[strings.TrimPrefix(a, "label="+LabelFleet+"=")], "\n")
			}
		}
	case args[0] == "image" && args[1] == "inspect":
		if d.missing[args[len(args)-1]] {
			return exec.CommandContext(ctx, "false")
		}
	}
	return exec.CommandContext(ctx, "printf", "%s", out)
}

// find returns every recorded call whose argv starts with prefix.
func (d *fakeDocker) find(prefix ...string) [][]string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out [][]string
	for _, c := range d.calls {
		if len(c) >= len(prefix) && strings.Join(c[:len(prefix)], "\x00") == strings.Join(prefix, "\x00") {
			out = append(out, c)
		}
	}
	return out
}

// installFakeDocker swaps commandContext and starts a banner server whose
// port the fake reports as the published port.
func installFakeDocker(t *testing.T) *fakeDocker {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("SSH-2.0-fake\r\n"))
			_ = c.Close()
		}
	}()
	d := &fakeDocker{port: ln.Addr().(*net.TCPAddr).Port, byFleet: map[string][]string{}, missing: map[string]bool{}}
	old := commandContext
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != "docker" {
			t.Errorf("simharness ran %q, only docker is allowed", name)
		}
		return d.reply(ctx, args)
	}
	t.Cleanup(func() { commandContext = old; _ = ln.Close() })
	t.Setenv(EnvIntegration, "1")
	return d
}

// flagValues returns every value that follows flag in args.
func flagValues(args []string, flag string) []string {
	var out []string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			out = append(out, args[i+1])
		}
	}
	return out
}
