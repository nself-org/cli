//go:build !unix

package licensing

import "os"

// Purpose: open a mounted token file; platforms without O_NOFOLLOW/O_NONBLOCK
// reject a symlink up front. Inputs: path. Outputs: open file or error.
// Constraints: best effort only, the caller checks the opened descriptor.
// SPORT: plugin.entitlement-token.
func openTokenFile(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrEntitlementExpired
	}
	return os.Open(path)
}
