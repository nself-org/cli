//go:build !unix

package secrets

import "os"

// storeHasMultipleLinks supplies the portable fallback for store checks.
// Inputs: regular-file metadata.
// Outputs: false where Unix link counts are unavailable.
// Constraints: other store path guards still apply. SPORT: D-0580.
func storeHasMultipleLinks(_ os.FileInfo) bool { return false }
