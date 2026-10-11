//go:build windows

package deploy

import (
	"os"
	"os/exec"
	"syscall"
)

// Windows has no process groups in this sense: nothing to do.
func detachProcessGroup(*exec.Cmd) {}

func interruptSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
