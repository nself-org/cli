package main

import (
	"bytes"
	"os"
	"os/exec"
)

// Runner runs an external command in the repo root and returns its stdout.
type Runner interface {
	Run(name string, args ...string) (string, error)
}

// ExecRunner is the production Runner. It sets CGO_ENABLED=0 like the scripts did.
type ExecRunner struct{ Dir string }

// Run executes name with args in e.Dir and returns stdout.
func (e ExecRunner) Run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = e.Dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return out.String(), err
}
