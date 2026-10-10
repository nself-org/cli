//go:build !windows

package invoke

// Purpose: run the child in its own process group and stop the whole group.
// Constraints: the child leads a new group (Setpgid), so a SIGTERM or SIGKILL
// to -pid reaches the grandchildren it started (docker compose, ssh, helpers)
// and not only the direct child. Windows has no process groups here: see
// procgroup_other.go.

import (
	"os"
	"os/exec"
	"syscall"
)

// ownGroup puts the child in a new process group.
func ownGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalGroup sends sig to the group led by p; if that fails it signals p.
func signalGroup(p *os.Process, sig syscall.Signal) error {
	if err := syscall.Kill(-p.Pid, sig); err != nil {
		return p.Signal(sig)
	}
	return nil
}

// killGroup SIGKILLs the group led by p. After the leader has been reaped the
// pid cannot be reused while a member of the group is alive, so this reaches
// exactly the leftover grandchildren; with none left it finds nothing.
func killGroup(p *os.Process) {
	_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
}
