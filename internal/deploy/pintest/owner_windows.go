//go:build windows

package pintest

import "os"

// owner is unknown on windows; the production check is skipped there too.
func owner(os.FileInfo) (int, bool) { return 0, false }
