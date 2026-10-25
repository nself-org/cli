//go:build darwin || linux

package docker

import (
	"os/exec"
	"syscall"
)

// setProcGroupAttr sets platform-specific SysProcAttr on cmd so that docker
// compose and all its children are placed in their own process group. This
// allows the Cancel hook to kill the entire group with a single signal.
func setProcGroupAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to the entire process group of cmd.
// On unix the pgid is the negation of the process PID when Setpgid is true.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// termProcessGroup sends SIGTERM to the entire process group of cmd: the docker
// CLI and the compose plugin it runs as a child both get it, so neither is
// orphaned when a stream is stopped.
func termProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}
