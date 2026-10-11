//go:build windows

package deploy

import "testing"

// statIno has no inode on Windows; the in-place restore test then compares
// equal zeros and the sh/tar/rsync tests skip themselves (needTools).
func statIno(t *testing.T, p string) uint64 { return 0 }
