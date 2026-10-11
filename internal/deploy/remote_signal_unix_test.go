//go:build !windows

package deploy

import (
	"os/exec"
	"syscall"
	"testing"
)

// G5: remote children start in their own process group, so a terminal SIGINT
// aimed at the operator's group does not kill an in-flight ssh or rsync.
func TestRemoteChildrenGetOwnProcessGroup(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	ownProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if pgid != cmd.Process.Pid || pgid == syscall.Getpgrp() {
		t.Errorf("child pgid %d, own pid %d, our pgid %d: want its own group", pgid, cmd.Process.Pid, syscall.Getpgrp())
	}
}
