package license

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Making the license directory usable when an older layout left a file there.
//
// Purpose: license paths under $HOME are created with MkdirAll. An older nSelf
// stored the license as a regular FILE at $HOME/.nself/license, with no
// extension. MkdirAll against that path returns ENOTDIR, so on an upgraded
// machine every license operation failed with a message naming neither the
// cause nor the fix:
//
//	Error: setting license key: remove /Users/<u>/.nself/license/key: not a directory
//
// MigrateLicenseFromV1 handles the separate `license.json` case and does not
// apply here.
//
// Inputs: a directory a caller is about to create.
//
// Outputs: nil once the path exists as a directory.
//
// Constraints: the migration is deliberately narrow. It fires ONLY for the
// license directory itself, because that is the one path a known older layout
// occupied with a file. Callers pass other directories here too, including
// $HOME/.nself, and silently renaming whatever sits at an arbitrary path would
// be far more destructive than the bug it fixes. Anything else that is not a
// directory is reported, not moved. The legacy file is never deleted: it is
// the user's license and may be the only copy.

// ensureDir creates dir. If the known legacy license file occupies the path it
// is moved aside first; any other non-directory is an error.
func ensureDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("checking %s: %w", dir, err)
		}
		return os.MkdirAll(dir, 0700)
	}
	if info.IsDir() {
		return nil
	}
	if !isLegacyLicenseFilePath(dir) {
		// Not the known legacy artifact. Say so plainly instead of renaming a
		// path this code has no business moving.
		return fmt.Errorf("%s exists but is not a directory; move or remove it and retry", dir)
	}
	// Keep it: this is the user's license data, and the problem is a layout
	// collision rather than a reason to discard it.
	moved := fmt.Sprintf("%s.legacy-%d", dir, time.Now().Unix())
	if rerr := os.Rename(dir, moved); rerr != nil {
		return fmt.Errorf("a file exists at %s where a directory is required, "+
			"and it could not be moved to %s: %w", dir, moved, rerr)
	}
	fmt.Fprintf(os.Stderr,
		"note: %s was a file from an older nSelf layout; moved to %s\n", dir, moved)
	return os.MkdirAll(dir, 0700)
}

// isLegacyLicenseFilePath reports whether dir is the license directory itself,
// the only path a previous layout is known to have occupied with a file.
func isLegacyLicenseFilePath(dir string) bool {
	return strings.HasSuffix(filepath.ToSlash(dir), "/"+licenseDir)
}
