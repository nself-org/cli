//go:build windows

package pluginperm

// Verify is a no-op on Windows: NT ACLs do not
// map to uid/gid bits, and plugin runtime on Windows is unverified (see
// runtime_windows.go). The security model here relies on the per-user
// profile directory being protected by the OS.
func Verify(_, _ string) error { return nil }
