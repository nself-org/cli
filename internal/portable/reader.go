package portable

import (
	"bytes"
	"encoding/json"
	"errors"
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
	for _, f := range m.Files {
		if err := verifyMember(root, f); err != nil {
			return nil, err
		}
	}
	return &Reader{root: root, m: m, byPath: byPath}, nil
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

// readManifest loads, version-checks, decodes and validates manifest.json.
func readManifest(root string, lim Limits) (Manifest, error) {
	p := filepath.Join(root, ManifestName)
	fi, err := os.Lstat(p)
	if err != nil {
		return Manifest{}, formatErr(ErrManifest, "%s not found: not a portable bundle", ManifestName)
	}
	if !fi.Mode().IsRegular() || linkCount(fi) > 1 {
		return Manifest{}, integrityErr(ErrLink, "%s is not a plain regular file", ManifestName)
	}
	if fi.Size() > lim.MaxManifestBytes {
		return Manifest{}, integrityErr(ErrSize, "%s is %d bytes, limit is %d", ManifestName, fi.Size(), lim.MaxManifestBytes)
	}
	fh, err := os.Open(p)
	if err != nil {
		return Manifest{}, formatErr(ErrManifest, "%s cannot be opened", ManifestName)
	}
	defer func() { _ = fh.Close() }()
	if st, err := fh.Stat(); err != nil || !os.SameFile(fi, st) {
		return Manifest{}, integrityErr(ErrLink, "%s changed while it was opened", ManifestName)
	}
	raw, err := io.ReadAll(io.LimitReader(fh, lim.MaxManifestBytes+1))
	if err != nil || int64(len(raw)) > lim.MaxManifestBytes {
		return Manifest{}, integrityErr(ErrSize, "%s cannot be read within %d bytes", ManifestName, lim.MaxManifestBytes)
	}
	var head struct {
		Format        string `json:"_format"`
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return Manifest{}, formatErr(ErrManifest, "%s is not a portable export manifest", ManifestName)
	}
	if head.Format != Format {
		return Manifest{}, formatErr(ErrUnknownMajor, "%s is not an nself portable export (_format %q)", ManifestName, truncate(head.Format))
	}
	if !versionRe.MatchString(head.SchemaVersion) {
		return Manifest{}, formatErr(ErrUnknownMajor,
			"bundle schema_version %q is not supported; this nself reads major %d", truncate(head.SchemaVersion), MajorVersion)
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, integrityErr(ErrManifest, "%s does not match the v1 layout: %v", ManifestName, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Manifest{}, integrityErr(ErrManifest, "%s has data after the manifest object", ManifestName)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, integrityErr(err, "bundle manifest is invalid: %v", err)
	}
	return m, nil
}

// truncate shortens an attacker-controlled string for an error message.
func truncate(s string) string {
	const max = 40
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
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

// Path returns the file system path of a listed member, for tools that need a
// file name (pg_restore). The bytes were verified at Open; call Verify again
// immediately before use if the directory could have changed since.
func (r *Reader) Path(rel string) (string, error) {
	if _, ok := r.byPath[rel]; !ok {
		return "", fmt.Errorf("portable: %q is not a member of this bundle", rel)
	}
	return filepath.Join(r.root, filepath.FromSlash(rel)), nil
}

// Open returns a reader for a listed member that re-checks length and SHA-256
// as it streams: reading to EOF yields E516 instead of io.EOF if the bytes
// changed since Open.
func (r *Reader) Open(rel string) (io.ReadCloser, error) {
	f, ok := r.byPath[rel]
	if !ok {
		return nil, fmt.Errorf("portable: %q is not a member of this bundle", rel)
	}
	fh, err := openMember(r.root, f)
	if err != nil {
		return nil, err
	}
	return newVerifyingReader(fh, f), nil
}

// Verify re-runs the member checks (links, length, SHA-256, unlisted entries).
func (r *Reader) Verify() error {
	if err := checkNoStrays(r.root, r.byPath, len(r.m.Files)); err != nil {
		return err
	}
	for _, f := range r.m.Files {
		if err := verifyMember(r.root, f); err != nil {
			return err
		}
	}
	return nil
}
