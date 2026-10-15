package firebase

// Purpose: mark this package's own constant error texts as safe to show the user.
// Inputs: constant strings. Outputs: errors whose text publicError may keep.
// Constraints: only constant text or an HTTP status number; never build one from a value read from
// the source (an id, a key, a URL, a response body).
import (
	"errors"

	"github.com/nself-org/cli/internal/importer"
)

// safeErr is an error with fixed, value-free text.
type safeErr struct{ msg string }

func (e safeErr) Error() string { return e.msg }

func safe(msg string) error { return safeErr{msg} }

// fail returns a user-visible extraction failure with fixed text.
func fail(msg string) error { return importer.NewSafe("E520", msg) }

// failWith adds err's text to prefix when err is one of this package's fixed-text errors; any
// other error (an OS error naming a path, a decoder error quoting input) shows the prefix alone.
func failWith(prefix string, err error) error {
	var s safeErr
	var st statusError
	if errors.As(err, &s) || errors.As(err, &st) {
		return fail(prefix + ": " + err.Error())
	}
	return fail(prefix)
}
