//go:build unix

package entitlement

import "syscall"

// The Unix open flag rejects symlinks before acquiring the writable handle.
const noFollowFlag = syscall.O_NOFOLLOW
