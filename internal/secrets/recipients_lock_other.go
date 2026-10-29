//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !windows

package secrets

import (
	"errors"
	"os"
)

func lockRecipientStore(string) (func(), error) {
	return nil, errors.New("recipient store locking is unsupported on this platform")
}

func multiplyLinked(os.FileInfo) bool { return false }
