package report

// tarball.go: writes the bundle as a 0600 tar.gz without overwriting anything.
//
// Purpose: publish the scrubbed bundle to its final path safely.
// Inputs: a Bundle, a destination path, a timestamp.
// Outputs: the file at the path (mode 0600 whatever the umask), its size.
// Constraints: an existing path is never overwritten (O_EXCL claims the name);
//         the archive is written to a temp file in the same directory and
//         renamed over the claimed name; failures remove both.

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultName is nself-report-<UTC timestamp>.tar.gz.
func DefaultName(now time.Time) string {
	return "nself-report-" + now.UTC().Format("20060102T150405Z") + ".tar.gz"
}

// WriteTarball writes b to path and returns the archive size in bytes.
func WriteTarball(b *Bundle, path string, now time.Time) (size int64, err error) {
	claim, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- caller-chosen output path
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return 0, fmt.Errorf("%s already exists: choose another --output (the report never overwrites)", path)
		}
		return 0, fmt.Errorf("creating %s: %w", path, err)
	}
	_ = claim.Close()
	tmp, err := os.CreateTemp(filepath.Dir(path), ".nself-report-*.tmp")
	if err != nil {
		_ = os.Remove(path)
		return 0, fmt.Errorf("creating temp file: %w", err)
	}
	done := false
	defer func() {
		if !done {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			_ = os.Remove(path)
		}
	}()
	if err = tmp.Chmod(0o600); err != nil {
		return 0, err
	}
	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)
	for _, f := range b.Files {
		hdr := &tar.Header{Name: f.Name, Mode: 0o600, Size: int64(len(f.Data)), ModTime: now.UTC(), Typeflag: tar.TypeReg}
		if err = tw.WriteHeader(hdr); err != nil {
			return 0, err
		}
		if _, err = tw.Write(f.Data); err != nil {
			return 0, err
		}
	}
	if err = tw.Close(); err != nil {
		return 0, err
	}
	if err = gz.Close(); err != nil {
		return 0, err
	}
	if err = tmp.Close(); err != nil {
		return 0, err
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return 0, fmt.Errorf("publishing %s: %w", path, err)
	}
	done = true
	if err = os.Chmod(path, 0o600); err != nil {
		return 0, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}
