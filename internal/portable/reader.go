package portable

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Limits bounds what Open accepts from an untrusted bundle. A zero field
// takes its DefaultLimits value.
type Limits struct {
	// MaxManifestBytes caps manifest.json.
	MaxManifestBytes int64
	// MaxFiles caps the number of listed members (and of directory entries).
	MaxFiles int
	// MaxFileBytes caps one member's declared length.
	MaxFileBytes int64
	// MaxTotalBytes caps the sum of declared member lengths.
	MaxTotalBytes int64
}

// DefaultLimits are generous for real exports and small enough that a forged
// manifest cannot make a reader allocate or hash without bound. Reading is
// always streamed and capped at the declared length, so a member can never
// inflate past what the manifest declares.
var DefaultLimits = Limits{
	MaxManifestBytes: 64 << 20,
	MaxFiles:         1_000_000,
	MaxFileBytes:     1 << 40,
	MaxTotalBytes:    8 << 40,
}

func (l Limits) withDefaults() Limits {
	if l.MaxManifestBytes <= 0 {
		l.MaxManifestBytes = DefaultLimits.MaxManifestBytes
	}
	if l.MaxFiles <= 0 {
		l.MaxFiles = DefaultLimits.MaxFiles
	}
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = DefaultLimits.MaxFileBytes
	}
	if l.MaxTotalBytes <= 0 {
		l.MaxTotalBytes = DefaultLimits.MaxTotalBytes
	}
	return l
}

// Reader is an opened, fully verified bundle.
type Reader struct {
	root   string
	m      Manifest
	byPath map[string]File
	snaps  map[string]snapshot // identity, size and mtime seen during verification
	lim    Limits
}

// Open reads and verifies the bundle in dir with DefaultLimits.
func Open(dir string) (*Reader, error) { return OpenWithLimits(dir, DefaultLimits) }

// OpenWithLimits is Open with explicit limits.
//
// Purpose: the bundle is untrusted input. Open refuses it unless every check
// passes, in this order: manifest.json is a regular file within the size
// limit; it is a v1 portable export (unknown format or major: E515); it
// parses and validates (E516); member count and sizes are within the limits;
// every member path is safe and unique; no entry exists that the manifest does
// not list; every member is a regular, singly linked file reached without a
// symlink, with the declared length and SHA-256 (E516, naming the file).
// Outputs: a Reader whose Manifest and members are verified at open time.
// Constraints: never prints bundle content; messages name paths only.
func OpenWithLimits(dir string, lim Limits) (*Reader, error) {
	lim = lim.withDefaults()
	root, err := resolveRoot(dir)
	if err != nil {
		return nil, err
	}
	m, err := readManifest(root, lim)
	if err != nil {
		return nil, err
	}
	byPath, err := checkLimits(m, lim)
	if err != nil {
		return nil, err
	}
	if err := checkNoStrays(root, byPath, lim.MaxFiles); err != nil {
		return nil, err
	}
	snaps := make(map[string]snapshot, len(m.Files))
	for _, f := range m.Files {
		snap, err := verifyMember(root, f, nil)
		if err != nil {
			return nil, err
		}
		snaps[f.Path] = snap
	}
	return &Reader{root: root, m: m, byPath: byPath, snaps: snaps, lim: lim}, nil
}

// resolveRoot returns the bundle directory with symlinks in the caller's own
// path resolved (the caller chose it); links inside the bundle are refused later.
func resolveRoot(dir string) (string, error) {
	if dir == "" {
		return "", formatErr(ErrManifest, "bundle directory is empty")
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", formatErr(ErrManifest, "bundle directory %q cannot be opened", dir)
	}
	fi, err := os.Stat(root)
	if err != nil || !fi.IsDir() {
		return "", formatErr(ErrManifest, "%q is not a bundle directory", dir)
	}
	return root, nil
}

// checkLimits applies the count and size limits and indexes the members.
func checkLimits(m Manifest, lim Limits) (map[string]File, error) {
	if len(m.Files) > lim.MaxFiles {
		return nil, integrityErr(ErrSize, "bundle lists %d files, limit is %d", len(m.Files), lim.MaxFiles)
	}
	byPath := make(map[string]File, len(m.Files))
	var total int64
	for _, f := range m.Files {
		if f.Bytes > lim.MaxFileBytes {
			return nil, integrityErr(ErrSize, "bundle file %q declares %d bytes, limit is %d", f.Path, f.Bytes, lim.MaxFileBytes)
		}
		if total > lim.MaxTotalBytes-f.Bytes {
			return nil, integrityErr(ErrSize, "bundle declares more than %d bytes in total", lim.MaxTotalBytes)
		}
		total += f.Bytes
		byPath[f.Path] = f
	}
	return byPath, nil
}

// Manifest returns the verified manifest.
func (r *Reader) Manifest() Manifest { return r.m }

// Dir returns the bundle directory (symlinks in the caller's path resolved).
func (r *Reader) Dir() string { return r.root }

// OpenFile opens a listed member and returns the descriptor, for tools that
// need a file rather than a stream (pg_restore reads standard input, or a
// /dev/fd path of this descriptor).
//
// Purpose: a path handed to another process can be swapped between the check
// and the use. This returns an open file instead, after re-checking that the
// member is still the file that was verified at Open: no symlink on the way
// (O_NOFOLLOW on the last component where the platform has it), one hard link,
// and the same device and inode (Windows: volume and file index), size and
// modification time as the snapshot taken during verification. Any difference
// is refused with E516 (ErrChanged, or ErrLink for a link).
// Constraints: the bytes are not hashed again; Open streams them with the
// length and SHA-256 check. Callers must close the file.
func (r *Reader) OpenFile(rel string) (*os.File, error) {
	f, ok := r.byPath[rel]
	if !ok {
		return nil, fmt.Errorf("portable: %q is not a member of this bundle", rel)
	}
	snap := r.snaps[rel]
	fh, _, err := openMember(r.root, f, &snap)
	return fh, err
}

// Open returns a reader for a listed member. It opens through OpenFile's
// checks and re-checks length and SHA-256 as it streams: reading to EOF yields
// E516 instead of io.EOF if the bytes changed since Open.
func (r *Reader) Open(rel string) (io.ReadCloser, error) {
	f, ok := r.byPath[rel]
	if !ok {
		return nil, fmt.Errorf("portable: %q is not a member of this bundle", rel)
	}
	fh, err := r.OpenFile(rel)
	if err != nil {
		return nil, err
	}
	return newVerifyingReader(fh, f), nil
}

// Verify re-runs the member checks (links, identity, length, SHA-256,
// unlisted entries) against the snapshot taken at Open.
func (r *Reader) Verify() error {
	if err := checkNoStrays(r.root, r.byPath, r.lim.MaxFiles); err != nil {
		return err
	}
	for _, f := range r.m.Files {
		snap := r.snaps[f.Path]
		if _, err := verifyMember(r.root, f, &snap); err != nil {
			return err
		}
	}
	return nil
}
