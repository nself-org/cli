//go:build !unix

package entitlement

import "os"

// Other platforms rely on Lstat and handle SameFile checks before any write.
const noFollowFlag = 0

func singleLink(info os.FileInfo) bool { return true }
