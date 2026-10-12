package hasurameta

// Purpose: add members to a finished portable bundle. portable.Writer seals a bundle at
// Finish, and the postgres source seals its own, so a source that adds storage objects or
// metadata copies the sealed bundle into a new one, edits the manifest, and seals that.
// Inputs: the sealed bundle directory, a new empty directory, an edit callback.
// Outputs: the new sealed bundle. Constraints: members stream through the verifying reader
// and the hashing writer (nothing is held in memory); the old bundle is the caller's to remove.
import (
	"fmt"
	"io"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/portable"
)

// Recompose copies every member of the sealed bundle in from into a new bundle in to, sets the
// producer source, calls add so it can write more members and change the manifest, then seals.
func Recompose(from, to, source string, add func(*portable.Writer, *portable.Manifest) error) error {
	r, err := portable.Open(from)
	if err != nil {
		return errs.New("E520", "extracted bundle failed integrity checks")
	}
	w, err := portable.NewWriter(to)
	if err != nil {
		return errs.New("E520", "bundle creation failed")
	}
	m := r.Manifest()
	for _, f := range m.Files {
		if err = copyMember(r, w, f.Path); err != nil {
			return errs.New("E520", "bundle member copy failed")
		}
	}
	m.Producer.Source = source
	if err = add(w, &m); err != nil {
		return err
	}
	if _, err = w.Finish(m); err != nil {
		return errs.New("E520", fmt.Sprintf("bundle manifest write failed: %v", err))
	}
	return nil
}

func copyMember(r *portable.Reader, w *portable.Writer, path string) error {
	rc, err := r.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = w.WriteFile(path, io.Reader(rc))
	return err
}
