package portable

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Patterns shared with the generated JSON schema.
const (
	// SHA256Pattern matches a lowercase hex SHA-256.
	SHA256Pattern = `^[0-9a-f]{64}$`
	// HashPattern matches a table hash in canonical form: a decimal integer
	// with no sign on zero, no leading zeros and no "+" ("0", "7", "-12").
	// Hashes are compared as strings, so one number has one spelling.
	HashPattern = `^(0|-?[1-9][0-9]*)$`
	// VersionPattern matches a v1 schema_version ("1", "1.2"): major 1 only.
	VersionPattern = `^1(\.[0-9]+)*$`
)

var (
	sha256Re  = regexp.MustCompile(SHA256Pattern)
	hashRe    = regexp.MustCompile(HashPattern)
	versionRe = regexp.MustCompile(VersionPattern)
)

// Validate checks the manifest against the v1 rules that the generated JSON
// schema cannot express alone: marker and version, RFC 3339 created_at,
// lowercase hex SHA-256 values, canonical table hashes, non-negative counts,
// unique tables, safe NFC member paths that are unique under NFC and case
// folding (so two members never land on one file of a case-insensitive or
// normalising file system), and storage objects that each name their own
// listed file with the same SHA-256 and length, with (bucket, key) unique and no
// file under storage/objects/ left without an object.
func (m *Manifest) Validate() error {
	err := m.validate()
	if err == nil || errors.Is(err, ErrUnsafePath) || errors.Is(err, ErrDuplicate) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrManifest, err)
}

// validate is Validate without the sentinel wrapping.
func (m *Manifest) validate() error {
	if m.Format != Format {
		return fmt.Errorf("_format is %q, want %q", m.Format, Format)
	}
	if !versionRe.MatchString(m.SchemaVersion) {
		return fmt.Errorf("schema_version %q is not major %d", m.SchemaVersion, MajorVersion)
	}
	if _, err := time.Parse(time.RFC3339, m.CreatedAt); err != nil {
		return fmt.Errorf("created_at %q is not RFC 3339", m.CreatedAt)
	}
	tables := make(map[[2]string]bool, len(m.DB.Tables))
	for _, t := range m.DB.Tables {
		if t.Rows < 0 || !hashRe.MatchString(t.Hash) {
			return fmt.Errorf("table %s.%s has an invalid row count or hash", t.Schema, t.Name)
		}
		k := [2]string{t.Schema, t.Name}
		if tables[k] {
			return fmt.Errorf("table %s.%s is listed twice: %w", t.Schema, t.Name, ErrDuplicate)
		}
		tables[k] = true
	}
	byPath, fold, err := m.checkFiles()
	if err != nil {
		return err
	}
	if err := checkParents(m.Files, fold); err != nil {
		return err
	}
	return m.checkObjects(byPath)
}

// checkFiles validates every files[] entry and returns them by exact path and
// the folded-name index.
func (m *Manifest) checkFiles() (map[string]File, map[string]string, error) {
	byPath := make(map[string]File, len(m.Files))
	fold := make(map[string]string, len(m.Files))
	for _, f := range m.Files {
		if err := CheckMember(f.Path); err != nil {
			return nil, nil, fmt.Errorf("file %q: %w", f.Path, err)
		}
		if f.Path == ManifestName {
			return nil, nil, fmt.Errorf("file %q: the manifest does not list itself", f.Path)
		}
		if f.Bytes < 0 || !sha256Re.MatchString(f.SHA256) {
			return nil, nil, fmt.Errorf("file %q has an invalid size or sha256", f.Path)
		}
		key := foldName(f.Path)
		if prev, dup := fold[key]; dup {
			return nil, nil, fmt.Errorf("file %q repeats member %q: %w", f.Path, prev, ErrDuplicate)
		}
		fold[key] = f.Path
		byPath[f.Path] = f
	}
	return byPath, fold, nil
}

// checkParents refuses a member that sits under another member (a file).
func checkParents(files []File, fold map[string]string) error {
	for _, f := range files {
		rest := f.Path
		for {
			i := strings.LastIndexByte(rest, '/')
			if i < 0 {
				break
			}
			rest = rest[:i]
			if other, clash := fold[foldName(rest)]; clash {
				return fmt.Errorf("file %q sits under member %q, which is a file", f.Path, other)
			}
		}
	}
	return nil
}

// checkObjects binds every storage object to its own listed file, and every
// file under storage/objects/ to an object (no unreferenced object bytes).
func (m *Manifest) checkObjects(byPath map[string]File) error {
	seen := make(map[[2]string]bool, len(m.Storage.Objects))
	used := make(map[string]bool, len(m.Storage.Objects))
	for _, o := range m.Storage.Objects {
		if o.Bytes < 0 || !sha256Re.MatchString(o.SHA256) {
			return fmt.Errorf("object %q/%q has an invalid size or sha256", o.Bucket, o.Key)
		}
		k := [2]string{o.Bucket, o.Key}
		if seen[k] {
			return fmt.Errorf("object %q/%q is listed twice: %w", o.Bucket, o.Key, ErrDuplicate)
		}
		seen[k] = true
		want, err := StorageMember(o.Bucket, o.Key)
		if err != nil {
			return fmt.Errorf("object %q/%q: %w", o.Bucket, o.Key, err)
		}
		if o.Member != want {
			return fmt.Errorf("object %q/%q names member %q, want %s", o.Bucket, o.Key, o.Member, want)
		}
		f, ok := byPath[o.Member]
		switch {
		case !ok:
			return fmt.Errorf("object %q/%q: member %s is not listed in files", o.Bucket, o.Key, o.Member)
		case f.SHA256 != o.SHA256 || f.Bytes != o.Bytes:
			return fmt.Errorf("object %q/%q: sha256 or bytes differ from file %s", o.Bucket, o.Key, o.Member)
		}
		used[o.Member] = true
	}
	for p := range byPath {
		if strings.HasPrefix(p, StorageMemberPrefix) && !used[p] {
			return fmt.Errorf("file %q is under %s but no storage.objects entry names it", p, StorageMemberPrefix)
		}
	}
	return nil
}
