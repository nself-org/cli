package reconcile

// Purpose: tell a caller that an apply failed after the generated files were
// published. Inputs: the failure; outputs: an error that matches ErrPublished.
// Constraints: only a failure that leaves the published files on disk carries
// the mark; a refusal or a rolled-back write never does.

import "errors"

// ErrPublished marks an Apply failure that happened after the generated files
// were written and left in place (the state record could not be written). A
// caller that installed plugins for this apply must keep them: the files
// reference them. Test with errors.Is.
var ErrPublished = errors.New("generated files were already published")

type publishedError struct{ err error }

func (p *publishedError) Error() string        { return p.err.Error() }
func (p *publishedError) Unwrap() error        { return p.err }
func (p *publishedError) Is(target error) bool { return target == ErrPublished }

// published marks err as a failure after publication; the message and the
// wrapped chain are unchanged. A nil err stays nil.
func published(err error) error {
	if err == nil {
		return nil
	}
	return &publishedError{err: err}
}
