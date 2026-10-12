//go:build !windows

package exporter

import "os"

// Purpose: atomically publish an owned directory. Inputs: stage and output.
// Outputs: rename error. Constraints: directory rename refuses a file,
// symlink or non-empty destination without an unlink-before-rename window.
func renameBundle(stage, out string) error { return os.Rename(stage, out) }
