//go:build windows

package envrestore

// owner_windows.go — keepOwner for the config set restore
// (P7-LIVE-05): Windows has no unix owner to keep.

import "os"

// keepOwner does nothing on Windows.
func keepOwner(string, os.FileInfo) error { return nil }
