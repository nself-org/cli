//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package secrets

import "errors"

func lockRecipientStore(string) (func(), error) {
	return nil, errors.New("recipient store locking is unsupported on this platform")
}
