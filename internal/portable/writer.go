package portable

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Writer builds a bundle directory. It is not safe for concurrent use.
//
// Purpose: one place that creates bundle files with the right modes and
// records each file's SHA-256 and length while it streams, so the manifest
// lists exactly the bytes that were written.
// Constraints: the bundle directory is 0700 and every file 0600 (EPIC D9).
// Files are created exclusively, so an existing file or a symlink in the way
// is an error, never followed. Member paths pass CheckMember.
type Writer struct {
	dir   string
	files map[string]File   // keyed by path
	fold  map[string]string // lowercase member path -> member path
	dirs  map[string]bool   // lowercase directory prefixes of members
	done  bool

	// Now supplies created_at when the manifest leaves it empty; tests pin it.
	Now func() time.Time
}

// NewWriter creates dir (0700) and returns a Writer for it. The directory must
// not exist, or must be empty; an existing non-empty directory is refused so a
// bundle never mixes with earlier content.
func NewWriter(dir string) (*Writer, error) {
	if dir == "" {
		return nil, errors.New("portable: bundle directory is empty")
	}
	if fi, err := os.Lstat(dir); err == nil {
		if !fi.IsDir() {
			return nil, fmt.Errorf("portable: %s exists and is not a directory", dir)
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("portable: read %s: %w", dir, err)
		}
		if len(ents) > 0 {
			return nil, fmt.Errorf("portable: %s is not empty", dir)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("portable: stat %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("portable: create %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("portable: chmod %s: %w", dir, err)
	}
	return &Writer{dir: dir, files: map[string]File{}, fold: map[string]string{}, dirs: map[string]bool{}, Now: time.Now}, nil
}

// Dir returns the bundle directory.
func (w *Writer) Dir() string { return w.dir }

// WriteFile streams r into the member rel and records its SHA-256 and length.
// rel must pass CheckMember, must not be manifest.json, and must not repeat an
// earlier member (compared case-insensitively) or collide with a member that
// is a file where this one needs a directory, or the reverse.
// On a copy error the partial file is removed.
func (w *Writer) WriteFile(rel string, r io.Reader) (File, error) {
	if w.done {
		return File{}, errors.New("portable: writer is finished")
	}
	if err := CheckMember(rel); err != nil {
		return File{}, err
	}
	if rel == ManifestName {
		return File{}, fmt.Errorf("%w: %s is written by Finish", ErrUnsafePath, ManifestName)
	}
	if err := w.reserve(rel); err != nil {
		return File{}, err
	}
	full := filepath.Join(w.dir, filepath.FromSlash(rel))
	if err := w.mkParents(filepath.Dir(full)); err != nil {
		return File{}, err
	}
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return File{}, fmt.Errorf("portable: create %s: %w", rel, err)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(full)
		return File{}, fmt.Errorf("portable: write %s: %w", rel, err)
	}
	rec := File{Path: rel, SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: n}
	w.files[rel] = rec
	return rec, nil
}

// reserve records rel as taken, refusing duplicates and file/directory clashes.
func (w *Writer) reserve(rel string) error {
	key := strings.ToLower(rel)
	if prev, dup := w.fold[key]; dup {
		return fmt.Errorf("portable: %w: %q repeats %q", ErrDuplicate, rel, prev)
	}
	for p := rel; ; {
		i := strings.LastIndexByte(p, '/')
		if i < 0 {
			break
		}
		p = p[:i]
		if prev, clash := w.fold[strings.ToLower(p)]; clash {
			return fmt.Errorf("portable: %q sits under member %q, which is a file", rel, prev)
		}
	}
	if w.dirs[key] {
		return fmt.Errorf("portable: member %q would be a file where earlier members need a directory", rel)
	}
	for p := key; ; {
		i := strings.LastIndexByte(p, '/')
		if i < 0 {
			break
		}
		p = p[:i]
		w.dirs[p] = true
	}
	w.fold[key] = rel
	return nil
}

// mkParents creates the directories above a member (0700), refusing any
// component that is a symlink or a file.
func (w *Writer) mkParents(dir string) error {
	rel, err := filepath.Rel(w.dir, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: parent outside the bundle", ErrUnsafePath)
	}
	cur := w.dir
	if rel == "." {
		return nil
	}
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		switch {
		case os.IsNotExist(err):
			if err := os.Mkdir(cur, 0o700); err != nil {
				return fmt.Errorf("portable: create directory: %w", err)
			}
		case err != nil:
			return fmt.Errorf("portable: stat directory: %w", err)
		case !fi.IsDir():
			return fmt.Errorf("%w: a parent of the member is not a directory", ErrUnsafePath)
		}
	}
	return nil
}

// Finish completes m, sorts it, validates it and writes manifest.json (0600).
// Format and SchemaVersion are set when empty and must be the v1 values
// otherwise; CreatedAt defaults to Now() in UTC; Files is replaced by the
// members written through WriteFile. The manifest bytes are canonical: sorted
// arrays, sorted map keys, two-space indent, no HTML escaping, one trailing
// newline; the same input gives the same bytes.
func (w *Writer) Finish(m Manifest) (Manifest, error) {
	if w.done {
		return Manifest{}, errors.New("portable: writer is finished")
	}
	if m.Format == "" {
		m.Format = Format
	}
	if m.SchemaVersion == "" {
		m.SchemaVersion = SchemaVersion
	}
	if m.CreatedAt == "" {
		m.CreatedAt = w.Now().UTC().Format(time.RFC3339)
	}
	m.Files = make([]File, 0, len(w.files))
	for _, f := range w.files {
		m.Files = append(m.Files, f)
	}
	m.Sort()
	if err := m.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("portable: %w", err)
	}
	b, err := MarshalManifest(m)
	if err != nil {
		return Manifest{}, err
	}
	f, err := os.OpenFile(filepath.Join(w.dir, ManifestName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Manifest{}, fmt.Errorf("portable: create %s: %w", ManifestName, err)
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(filepath.Join(w.dir, ManifestName))
		return Manifest{}, fmt.Errorf("portable: write %s: %w", ManifestName, err)
	}
	w.done = true
	return m, nil
}

// MarshalManifest returns the canonical manifest.json bytes of m (m should
// already be sorted).
func MarshalManifest(m Manifest) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, fmt.Errorf("portable: encode manifest: %w", err)
	}
	return buf.Bytes(), nil
}
