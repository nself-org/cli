//go:build !unix

package entitlement

// Other platforms rely on Lstat and handle SameFile checks before any write.
const noFollowFlag = 0
