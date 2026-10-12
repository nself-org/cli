//go:build !windows

package build

// Purpose: tell, before anything is planned or deleted, whether this user can
// unlink inside a directory (the traefik-config one-shot runs as root, so Docker
// creates its bind directories root-owned on Linux).
// Inputs: a directory path. Outputs: whether entries can be created and removed.
// Constraints: read-only (access(2)), identical in plan and apply mode.

import "syscall"

func dirWritable(dir string) bool { return syscall.Access(dir, 0x2|0x1) == nil } // W_OK|X_OK
