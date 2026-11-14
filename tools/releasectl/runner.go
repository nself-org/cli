package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Runner executes an external command and returns its stdout.
// Every external call (gh, git) goes through it so tests can fake it.
type Runner interface {
	Run(name string, args ...string) (stdout string, err error)
}

// ExecRunner is the real Runner backed by os/exec.
type ExecRunner struct {
	// Stderr receives the child's stderr; nil means os.Stderr.
	Stderr io.Writer
}

// Run executes name with args and returns captured stdout.
func (e ExecRunner) Run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	var dst io.Writer = os.Stderr
	if e.Stderr != nil {
		dst = e.Stderr
	}
	cmd.Stderr = io.MultiWriter(dst, &errb)
	err := cmd.Run()
	if err != nil {
		code := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
		err = &RunError{Code: code, Stderr: errb.String()}
	}
	return out.String(), err
}

// RunError is the failure of a command run through a Runner that reports its
// exit code and captured stderr, so callers can tell "404" from "unreachable".
type RunError struct {
	Code   int    // exit status, -1 when unknown
	Stderr string // what the command wrote to stderr
}

// Error describes the failure.
func (e *RunError) Error() string {
	return fmt.Sprintf("command failed (exit %d): %s", e.Code, e.Stderr)
}
