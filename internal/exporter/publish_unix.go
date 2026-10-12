//go:build !windows

package exporter

import "syscall"

// Purpose: atomically publish an owned directory. Inputs: stage and output.
// Outputs: rename error. Constraints: calls rename(2) directly because
// os.Rename refuses any existing directory destination; the kernel replaces
// an empty directory and refuses a file, symlink or non-empty directory
// without an unlink-before-rename window.
func renameBundle(stage, out string) error { return syscall.Rename(stage, out) }
