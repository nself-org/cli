package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
)

// DirRunner is a Runner that can also run a command in a working directory,
// optionally on the owner's terminal. The release steps use it for the
// `(cd DIR && cmd)` lines of the shell; a Runner without it fails those calls.
type DirRunner interface {
	Runner
	// RunIn runs name in dir. With tty the child inherits stdin, stdout and
	// stderr, so admin-merge.sh can ask the owner at the terminal.
	RunIn(dir string, tty bool, name string, args ...string) (string, error)
}

// RunIn runs name in dir; see DirRunner.
func (e ExecRunner) RunIn(dir string, tty bool, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if tty {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return "", &RunError{Code: exitCode(err)}
		}
		return "", nil
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	var dst io.Writer = os.Stderr
	if e.Stderr != nil {
		dst = e.Stderr
	}
	cmd.Stderr = io.MultiWriter(dst, &errb)
	if err := cmd.Run(); err != nil {
		return out.String(), &RunError{Code: exitCode(err), Stderr: errb.String()}
	}
	return out.String(), nil
}

func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// runIn runs a command in dir through the Runner, or fails when it cannot.
func (r *Release) runIn(dir string, tty bool, name string, args ...string) (string, error) {
	if d, ok := r.Run.(DirRunner); ok {
		return d.RunIn(dir, tty, name, args...)
	}
	return "", &RunError{Code: -1, Stderr: "this Runner cannot run a command in a directory"}
}
