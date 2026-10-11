//go:build !windows

package deploy

import (
	"os/exec"
	"syscall"
)

func detachProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
