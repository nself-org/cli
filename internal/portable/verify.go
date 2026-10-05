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

// openMember opens the listed member f under root without following links and
// returns the descriptor with the snapshot taken from it.
//
// Every directory component and the file itself are Lstat-ed: a symlink
// anywhere, a non-regular file or a file with more than one hard link is
// refused; the length must equal the manifest's. The file is opened with
// O_NOFOLLOW where the platform has it, and the opened descriptor must be the
// file that was Lstat-ed (a swap between the check and the open is refused).
// When want is not nil the descriptor must also be the snapshot taken when the
// bundle was verified: same device and inode (Windows: volume and file index),
// size and modification time, or the open is refused with ErrChanged.
func openMember(root string, f File, want *snapshot) (*os.File, snapshot, error) {
	cur := root
	segs := strings.Split(f.Path, "/")
	for i, seg := range segs {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			return nil, snapshot{}, integrityErr(ErrMissing, "bundle file %q is missing", f.Path)
		}
		if err != nil {
			return nil, snapshot{}, integrityErr(ErrMissing, "bundle file %q cannot be read: %v", f.Path, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, snapshot{}, integrityErr(ErrLink, "bundle member %q goes through a symlink", f.Path)
		}
		if i < len(segs)-1 {
			if !fi.IsDir() {
				return nil, snapshot{}, integrityErr(ErrLink, "bundle member %q has a non-directory parent", f.Path)
			}
			continue
		}
		if !fi.Mode().IsRegular() {
			return nil, snapshot{}, integrityErr(ErrLink, "bundle member %q is not a regular file", f.Path)
		}
		return openChecked(cur, fi, f, want)
	}
	return nil, snapshot{}, integrityErr(ErrMissing, "bundle file %q is missing", f.Path)
}

// openChecked opens path (already Lstat-ed as fi) and applies the descriptor
// checks of openMember.
func openChecked(path string, fi os.FileInfo, f File, want *snapshot) (*os.File, snapshot, error) {
	fh, err := os.OpenFile(path, openFlags, 0)
	if err != nil {
		return nil, snapshot{}, integrityErr(ErrLink, "bundle file %q cannot be opened without following links: %v", f.Path, err)
	}
	snap, nlink, st, err := snapshotOf(fh)
	switch {
	case err != nil:
		_ = fh.Close()
		return nil, snapshot{}, integrityErr(ErrMissing, "bundle file %q cannot be inspected: %v", f.Path, err)
	case !st.Mode().IsRegular() || !os.SameFile(fi, st):
		_ = fh.Close()
		return nil, snapshot{}, integrityErr(ErrLink, "bundle file %q changed while it was opened", f.Path)
	case nlink > 1:
		_ = fh.Close()
		return nil, snapshot{}, integrityErr(ErrLink, "bundle member %q is a hard link", f.Path)
	case snap.size != f.Bytes:
		_ = fh.Close()
		return nil, snapshot{}, integrityErr(ErrSize, "bundle file %q is %d bytes, manifest says %d", f.Path, snap.size, f.Bytes)
	case want != nil && !want.equal(snap):
		_ = fh.Close()
		return nil, snapshot{}, integrityErr(ErrChanged, "bundle file %q is not the file that was verified (identity, size or modification time differs)", f.Path)
	}
	return fh, snap, nil
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

// verifyMember reads one member to the end, checks its length and SHA-256,
// and returns its snapshot. When want is not nil the member must also be that
// snapshot. The descriptor is inspected again after the read: a member that
// was written to while it was hashed is refused.
func verifyMember(root string, f File, want *snapshot) (snapshot, error) {
	fh, snap, err := openMember(root, f, want)
	if err != nil {
		return snapshot{}, err
	}
	vr := newVerifyingReader(fh, f)
	defer func() { _ = vr.Close() }()
	if _, err := io.Copy(io.Discard, vr); err != nil {
		return snapshot{}, err
	}
	after, _, _, err := snapshotOf(fh)
	if err != nil || !after.equal(snap) {
		return snapshot{}, integrityErr(ErrChanged, "bundle file %q changed while it was being verified", f.Path)
	}
	return snap, nil
}

// checkNoStrays refuses any entry of the bundle directory that the manifest
// does not list: an unlisted file or symlink is never read by a consumer, but
// a bundle that carries one is not the bundle that was exported. Directories
// are allowed (they hold listed files) but count, with files, toward limit
// (MaxFiles), so a tree of empty directories cannot make the walk unbounded.
func checkNoStrays(root string, listed map[string]File, limit int) error {
	seen := 0
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return integrityErr(ErrMissing, "bundle directory cannot be read: %v", err)
		}
		if p == root {
			return nil
		}
		if seen++; seen > limit+1 {
			return integrityErr(ErrSize, "bundle directory holds more than %d entries", limit)
		}
		if d.IsDir() && d.Type()&os.ModeSymlink == 0 {
			return nil
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
