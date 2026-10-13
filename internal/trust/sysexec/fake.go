package sysexec

import (
	"context"
	"os/exec"
	"strings"
	"sync"
)

// Call records one command a Fake saw.
type Call struct {
	Name string
	Args []string
}

// String renders the call as a single command line.
func (c Call) String() string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

// Fake is a Runner for tests: it records every call and answers from Reply.
// It never executes anything. Reply may be nil (every command succeeds with no
// output).
type Fake struct {
	mu    sync.Mutex
	calls []Call
	// Reply decides the result for a command line (see Call.String). Return
	// (nil, nil) for plain success.
	Reply func(c Call) ([]byte, error)
	// Missing lists binaries LookPath reports as not installed. Everything
	// else is reported as installed under /fake/bin.
	Missing map[string]bool
}

// Calls returns a copy of the recorded calls in order.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Names returns just the command names, in order.
func (f *Fake) Names() []string {
	var out []string
	for _, c := range f.Calls() {
		out = append(out, c.Name)
	}
	return out
}

// Saw reports whether any recorded command line contains substr.
func (f *Fake) Saw(substr string) bool {
	for _, c := range f.Calls() {
		if strings.Contains(c.String(), substr) {
			return true
		}
	}
	return false
}

func (f *Fake) record(name string, args []string) ([]byte, error) {
	c := Call{Name: name, Args: append([]string(nil), args...)}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	reply := f.Reply
	f.mu.Unlock()
	if reply == nil {
		return nil, nil
	}
	return reply(c)
}

// Output implements Runner.
func (f *Fake) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	return f.record(name, args)
}

// Stdout implements Runner.
func (f *Fake) Stdout(_ context.Context, name string, args ...string) ([]byte, error) {
	return f.record(name, args)
}

// Stream implements Runner.
func (f *Fake) Stream(_ context.Context, name string, args ...string) error {
	_, err := f.record(name, args)
	return err
}

// LookPath implements Runner.
func (f *Fake) LookPath(name string) (string, error) {
	if f.Missing[name] {
		return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
	return "/fake/bin/" + name, nil
}
