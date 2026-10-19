//go:build !unix

package licensing

import "os"

// Purpose: open the entitlement file where O_NOFOLLOW is unavailable.
// Inputs: path. Outputs: open regular file or ErrEntitlementExpired.
// Constraints: the opened descriptor must be the file Lstat checked. SPORT: plugin.entitlement-token.
func openRegular(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, ErrEntitlementExpired
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrEntitlementExpired
	}
	after, err := f.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = f.Close()
		return nil, ErrEntitlementExpired
	}
	return f, nil
}
