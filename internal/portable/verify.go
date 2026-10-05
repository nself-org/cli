package portable

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// openMember opens the listed member f under root without following links.
//
// Every directory component and the file itself are Lstat-ed: a symlink
// anywhere, a non-regular file or a file with more than one hard link is
// refused; the length must equal the manifest's; and the opened descriptor
// must be the same file that was Lstat-ed (a swap between the check and the
// open is refused).
func openMember(root string, f File) (*os.File, error) {
	cur := root
	segs := strings.Split(f.Path, "/")
	for i, seg := range segs {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			return nil, integrityErr(ErrMissing, "bundle file %q is missing", f.Path)
		}
		if err != nil {
			return nil, integrityErr(ErrMissing, "bundle file %q cannot be read: %v", f.Path, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, integrityErr(ErrLink, "bundle member %q goes through a symlink", f.Path)
		}
		if i < len(segs)-1 {
			if !fi.IsDir() {
				return nil, integrityErr(ErrLink, "bundle member %q has a non-directory parent", f.Path)
			}
			continue
		}
		if !fi.Mode().IsRegular() {
			return nil, integrityErr(ErrLink, "bundle member %q is not a regular file", f.Path)
		}
		if linkCount(fi) > 1 {
			return nil, integrityErr(ErrLink, "bundle member %q is a hard link", f.Path)
		}
		if fi.Size() != f.Bytes {
			return nil, integrityErr(ErrSize, "bundle file %q is %d bytes, manifest says %d", f.Path, fi.Size(), f.Bytes)
		}
		fh, err := os.Open(cur)
		if err != nil {
			return nil, integrityErr(ErrMissing, "bundle file %q cannot be opened: %v", f.Path, err)
		}
		st, err := fh.Stat()
		if err != nil || !os.SameFile(fi, st) {
			_ = fh.Close()
			return nil, integrityErr(ErrLink, "bundle file %q changed while it was opened", f.Path)
		}
		return fh, nil
	}
	return nil, integrityErr(ErrMissing, "bundle file %q is missing", f.Path)
}

// verifyingReader streams one member and checks its length and SHA-256.
//
// It never returns more than the manifest's length. At end of file it returns
// E516 instead of io.EOF when the length or hash differs, so a consumer that
// reads to EOF cannot accept altered bytes. Bytes returned before EOF are
// unverified until that point.
type verifyingReader struct {
	f    *os.File
	want File
	h    hash.Hash
	n    int64
	err  error
}

func newVerifyingReader(f *os.File, want File) *verifyingReader {
	return &verifyingReader{f: f, want: want, h: sha256.New()}
}

func (v *verifyingReader) Read(p []byte) (int, error) {
	if v.err != nil {
		return 0, v.err
	}
	if room := v.want.Bytes - v.n + 1; int64(len(p)) > room {
		p = p[:room] // read at most one byte past the declared length
	}
	n, err := v.f.Read(p)
	v.n += int64(n)
	if v.n > v.want.Bytes {
		v.err = integrityErr(ErrSize, "bundle file %q is longer than the manifest says (%d bytes)", v.want.Path, v.want.Bytes)
		return 0, v.err
	}
	v.h.Write(p[:n])
	if errors.Is(err, io.EOF) {
		switch {
		case v.n != v.want.Bytes:
			v.err = integrityErr(ErrSize, "bundle file %q is %d bytes, manifest says %d", v.want.Path, v.n, v.want.Bytes)
		case hex.EncodeToString(v.h.Sum(nil)) != v.want.SHA256:
			v.err = integrityErr(ErrChecksum, "bundle file %q does not match its sha256 in the manifest", v.want.Path)
		default:
			v.err = io.EOF
		}
		return n, v.err
	}
	return n, err
}

func (v *verifyingReader) Close() error { return v.f.Close() }

// verifyMember reads one member to the end and checks it.
func verifyMember(root string, f File) error {
	fh, err := openMember(root, f)
	if err != nil {
		return err
	}
	vr := newVerifyingReader(fh, f)
	defer func() { _ = vr.Close() }()
	if _, err := io.Copy(io.Discard, vr); err != nil {
		return err
	}
	return nil
}

// checkNoStrays refuses any entry of the bundle directory that the manifest
// does not list: an unlisted file or symlink is never read by a consumer, but
// a bundle that carries one is not the bundle that was exported. Directories
// are allowed (they hold listed files). limit bounds the entries visited.
func checkNoStrays(root string, listed map[string]File, limit int) error {
	seen := 0
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return integrityErr(ErrMissing, "bundle directory cannot be read: %v", err)
		}
		if p == root || (d.IsDir() && d.Type()&os.ModeSymlink == 0) {
			return nil
		}
		if seen++; seen > limit+1 {
			return integrityErr(ErrSize, "bundle directory holds more than %d entries", limit)
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return integrityErr(ErrUnlisted, "bundle entry outside the bundle directory")
		}
		rel = filepath.ToSlash(rel)
		if rel == ManifestName {
			return nil
		}
		if _, ok := listed[rel]; ok {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return integrityErr(ErrLink, "bundle holds a symlink %q", rel)
		}
		return integrityErr(ErrUnlisted, "bundle holds %q, which the manifest does not list", rel)
	})
}
