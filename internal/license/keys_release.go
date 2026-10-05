//go:build !nself_devkeys

// Package license — keys_release.go: the key set of every normal build.
//
// Purpose: release and default builds (goreleaser, go install, go test) verify
// ping's signatures with the committed PingKeys only.
// Constraints: no environment variable and no linker flag adds a key here
// (review R2). Dev keys exist only in builds tagged nself_devkeys.
package license

// devKeysBuild reports whether this binary was built with the nself_devkeys tag.
const devKeysBuild = false

// extraKeys returns no keys; it never reads the environment.
func extraKeys() []PublicKeyEntry { return nil }

// resetDevKeyWarning is a no-op: release builds print no dev-key warning.
func resetDevKeyWarning() {}
