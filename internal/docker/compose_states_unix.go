//go:build !windows

package docker

// compose_states_unix.go — soft cancel for bounded docker reads.

import (
	"os/exec"
	"syscall"
)

// softCancel makes a cancelled ctx send SIGTERM instead of SIGKILL: the docker
// CLI forwards it to the compose plugin it runs as a child, so the plugin is
// not orphaned. WaitDelay still escalates to a kill if docker ignores it.
func softCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
}
