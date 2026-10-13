// Package sysexec is the single door through which the trust, SSL and DNS
// setup code reaches the operating system: it runs external commands and says
// whether the process is headless.
//
// Purpose: keep privileged system changes (osascript admin prompts, sudo,
// pfctl, launchctl, brew, security, iptables, systemctl) behind one injectable
// runner, so unit tests never touch the real trust store, /etc/hosts,
// /etc/resolver or pf, and so a headless host (CI, automation, containers)
// can skip every system change instead of hanging on a password dialog.
// Inputs: command name + args; the NSELF_HEADLESS and CI environment variables.
// Outputs: combined command output, or ErrSkippedHeadless from callers that
// were about to change the system.
// Constraints: stdlib only; imported by internal/trust, internal/ssl and
// cmd/commands, so it must never import any of them.
package sysexec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrSkippedHeadless is returned (possibly wrapped) by a setup step that found
// its state missing but did not change the system because the process is
// headless. It is a clear "skipped (headless)" result, not a failure.
var ErrSkippedHeadless = errors.New("skipped (headless): set NSELF_HEADLESS=0 to allow admin prompts, or run the step manually")

// Headless reports whether system changes must be skipped. It is true when
// NSELF_HEADLESS is truthy, or, unless NSELF_HEADLESS is explicitly falsy,
// when CI is truthy. Accepted truthy values: "1", "true", "yes", "on"
// (case-insensitive). Falsy: "0", "false", "no", "off".
func Headless() bool {
	switch truth(os.Getenv("NSELF_HEADLESS")) {
	case triTrue:
		return true
	case triFalse:
		return false
	}
	return truth(os.Getenv("CI")) == triTrue
}

type tri int

const (
	triUnset tri = iota
	triTrue
	triFalse
)

func truth(v string) tri {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return triTrue
	case "0", "false", "no", "off":
		return triFalse
	}
	return triUnset
}

// Runner executes external commands. Production uses the guarded exec runner;
// tests install a fake with Set.
type Runner interface {
	// Output runs the command and returns its combined stdout+stderr.
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	// Stdout runs the command and returns only its stdout (stderr is dropped),
	// for output that is written to a file, such as iptables-save.
	Stdout(ctx context.Context, name string, args ...string) ([]byte, error)
	// Stream runs the command with stdout and stderr wired to the process's
	// own, for long installs where the operator wants to see progress.
	Stream(ctx context.Context, name string, args ...string) error
	// LookPath reports where name is installed, like exec.LookPath.
	LookPath(name string) (string, error)
}

var (
	mu      sync.RWMutex
	current Runner = execRunner{}
)

// Real returns the production exec runner, guard included. Tests use it to
// prove the guard stops privileged commands even when real code is driven.
func Real() Runner { return execRunner{} }

// Current returns the active runner.
func Current() Runner {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Set installs r as the active runner and returns a function that restores the
// previous one. It is meant for tests.
func Set(r Runner) (restore func()) {
	mu.Lock()
	prev := current
	current = r
	mu.Unlock()
	return func() {
		mu.Lock()
		current = prev
		mu.Unlock()
	}
}

// Output runs a command through the active runner.
func Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return Current().Output(ctx, name, args...)
}

// Stream runs a command through the active runner with inherited stdio.
func Stream(ctx context.Context, name string, args ...string) error {
	return Current().Stream(ctx, name, args...)
}

// Stdout runs a command through the active runner and returns its stdout only.
func Stdout(ctx context.Context, name string, args ...string) ([]byte, error) {
	return Current().Stdout(ctx, name, args...)
}

// LookPath finds a binary through the active runner.
func LookPath(name string) (string, error) {
	return Current().LookPath(name)
}

// Run runs a command through the active runner and discards its output.
func Run(ctx context.Context, name string, args ...string) error {
	_, err := Current().Output(ctx, name, args...)
	return err
}

// osascriptWaitDelay bounds how long a cancelled osascript may keep its I/O
// pipes open before Go gives up on draining them (the admin dialog can leave
// helpers behind). Other commands keep exec's default behaviour.
const osascriptWaitDelay = 5 * time.Second

func applyWaitDelay(cmd *exec.Cmd, name string) {
	if filepath.Base(name) == "osascript" {
		cmd.WaitDelay = osascriptWaitDelay
	}
}

// execRunner is the production runner. When the binary is a Go test binary it
// first applies the privileged-command guard (see guard.go).
type execRunner struct{}

func (execRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	guard(name, args)
	cmd := exec.CommandContext(ctx, name, args...)
	applyWaitDelay(cmd, name)
	return cmd.CombinedOutput()
}

func (execRunner) Stdout(ctx context.Context, name string, args ...string) ([]byte, error) {
	guard(name, args)
	cmd := exec.CommandContext(ctx, name, args...)
	applyWaitDelay(cmd, name)
	return cmd.Output()
}

func (execRunner) Stream(ctx context.Context, name string, args ...string) error {
	guard(name, args)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	applyWaitDelay(cmd, name)
	return cmd.Run()
}

func (execRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }
