//go:build !unix

package secrets

import "os"

// storeHasMultipleLinks retains the regular-file guard on other platforms.
func storeHasMultipleLinks(_ os.FileInfo) bool { return false }
