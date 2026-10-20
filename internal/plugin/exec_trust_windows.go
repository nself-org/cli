//go:build windows

package plugin

// execTrustProblem is a no-op on Windows: file ownership and mode bits do not
// map to the unix model the check relies on (access is by ACL).
func execTrustProblem(resolved, resolvedRoot string, uid int) string { return "" }
