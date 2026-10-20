//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package secrets

// lockRecipientStore fails closed where a per-store flock is unavailable.
func lockRecipientStore(_ string) (func(), error) { return nil, decryptError() }
