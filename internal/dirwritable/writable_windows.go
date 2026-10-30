//go:build windows

package dirwritable

// writable has no access(2) on Windows; a failed removal is reported by the
// remove itself, which the build cleanup treats the same way.
func writable(string) bool { return true }
