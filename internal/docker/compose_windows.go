//go:build windows

package docker

import "os/exec"

// setProcGroupAttr is a no-op on Windows. Windows does not support POSIX
// process groups; job objects would be the equivalent but are not needed for
// the current use case — cmd.Process.Kill() terminates the immediate process
// and Docker Desktop handles child cleanup.
func setProcGroupAttr(_ *exec.Cmd) {}

// killProcessGroup is a no-op on Windows because there is no unix-style
// process group to signal. The Cancel hook falls through to cmd.Process.Kill().
func killProcessGroup(_ *exec.Cmd) {}

// termProcessGroup kills the immediate process: Windows has no SIGTERM to send.
func termProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
