package portable

import (
	"errors"
	"fmt"

	"github.com/nself-org/cli/internal/errs"
)

// Sentinels wrapped inside the E515/E516 errors, for errors.Is.
var (
	// ErrUnknownMajor: the manifest is not a v1 portable export (E515).
	ErrUnknownMajor = errors.New("unknown portable bundle format or major version")
	// ErrChecksum: a member's SHA-256 differs from the manifest (E516).
	ErrChecksum = errors.New("bundle file checksum mismatch")
	// ErrMissing: a listed member is absent (E516).
	ErrMissing = errors.New("bundle file missing")
	// ErrSize: a member's length differs from the manifest, or a size limit
	// was exceeded (E516).
	ErrSize = errors.New("bundle file size mismatch or limit exceeded")
	// ErrDuplicate: two members share a path (E516).
	ErrDuplicate = errors.New("duplicate bundle member")
	// ErrUnlisted: an entry exists in the bundle directory that the manifest
	// does not list (E516).
	ErrUnlisted = errors.New("bundle entry not listed in the manifest")
	// ErrLink: a member is a symlink, a hard link or not a regular file (E516).
	ErrLink = errors.New("bundle member is a link or not a regular file")
	// ErrChanged: a member is not the file that was verified at Open (another
	// file, or the same file with a different size or modification time) (E516).
	ErrChanged = errors.New("bundle file changed since it was verified")
	// ErrManifest: manifest.json is unreadable or fails validation (E516 for a
	// v1 manifest, E515 when it is not a v1 manifest at all).
	ErrManifest = errors.New("invalid bundle manifest")
)

// formatErr builds the E515 error.
func formatErr(sentinel error, format string, args ...any) error {
	return errs.Wrap("E515", fmt.Sprintf(format, args...), sentinel)
}

// integrityErr builds the E516 error; the message names the member.
func integrityErr(sentinel error, format string, args ...any) error {
	return errs.Wrap("E516", fmt.Sprintf(format, args...), sentinel)
}
