//go:build windows

package build

// dirWritable has no access(2) on Windows; a failed removal is reported by the
// remove itself, which cleanupOutputDir treats the same way.
func dirWritable(string) bool { return true }
