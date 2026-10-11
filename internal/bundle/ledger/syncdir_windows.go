//go:build windows

package ledger

// Purpose: Windows build stub of syncDir. A native Windows build cannot fsync a
// directory handle; the rename itself is the only durability step there.
// Windows is supported through WSL2 (VMI), where the unix build runs.

func syncDir(string) error { return nil }
