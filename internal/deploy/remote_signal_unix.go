//go:build !windows

package deploy

import (
	"os"
	"os/exec"
	"syscall"
)

func detachProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// interruptSignals are the signals that cancel a remote apply: Ctrl-C, kill and
// the hangup of a closed terminal or ssh session.
func interruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
}
