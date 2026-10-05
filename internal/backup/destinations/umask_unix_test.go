//go:build !windows

package destinations

import "syscall"

func syscallUmask(m int) int { return syscall.Umask(m) }
