//go:build windows

package invoke

// Purpose: the no-op counterpart of procgroup_unix.go. A Windows child cannot
// be asked to stop with a signal, so cancel kills the child and nothing more.

import (
	"os"
	"os/exec"
	"syscall"
)

func ownGroup(*exec.Cmd) {}

func signalGroup(p *os.Process, _ syscall.Signal) error { return p.Kill() }

func killGroup(*os.Process) {}
