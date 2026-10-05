package portable

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Patterns shared with the generated JSON schema.
const (
	// SHA256Pattern matches a lowercase hex SHA-256.
	SHA256Pattern = `^[0-9a-f]{64}$`
	// HashPattern matches a table hash: a decimal integer string.
	HashPattern = `^-?[0-9]+$`
	// VersionPattern matches a v1 schema_version ("1", "1.2"): major 1 only.
	VersionPattern = `^1(\.[0-9]+)*$`
)

var (
	sha256Re  = regexp.MustCompile(SHA256Pattern)
	hashRe    = regexp.MustCompile(HashPattern)
	versionRe = regexp.MustCompile(VersionPattern)
)

// Sort puts every array into its canonical order and replaces nil slices by
// empty ones, so the marshalled manifest is the same for equal content.
//
// Purpose: Constitution 4.4 deterministic output (sorted keys and arrays).
// Constraints: PK keeps the key's column order (it is data, not a set);
// maps need no work because encoding/json marshals them in key order.
func (m *Manifest) Sort() {
	m.DB.Schemas = sortedStrings(m.DB.Schemas)
	if m.DB.Tables == nil {
		m.DB.Tables = []Table{}
	}
	sort.SliceStable(m.DB.Tables, func(i, j int) bool {
		a, b := m.DB.Tables[i], m.DB.Tables[j]
		return a.Schema < b.Schema || (a.Schema == b.Schema && a.Name < b.Name)
	})
	for i := range m.DB.Tables {
		if m.DB.Tables[i].PK == nil {
			m.DB.Tables[i].PK = []string{}
		}
	}
	if m.Auth.HashAlgorithms == nil {
		m.Auth.HashAlgorithms = map[string]int{}
	}
	m.Auth.ResetRequired = sortedStrings(m.Auth.ResetRequired)
	if m.Storage.Buckets == nil {
		m.Storage.Buckets = []Bucket{}
	}
	sort.SliceStable(m.Storage.Buckets, func(i, j int) bool { return m.Storage.Buckets[i].Name < m.Storage.Buckets[j].Name })
	if m.Storage.Objects == nil {
		m.Storage.Objects = []Object{}
	}
	sort.SliceStable(m.Storage.Objects, func(i, j int) bool {
		a, b := m.Storage.Objects[i], m.Storage.Objects[j]
		return a.Bucket < b.Bucket || (a.Bucket == b.Bucket && a.Key < b.Key)
	})
	if m.Exemptions == nil {
		m.Exemptions = []Exemption{}
	}
	sort.SliceStable(m.Exemptions, func(i, j int) bool {
		a, b := m.Exemptions[i], m.Exemptions[j]
		return a.Kind < b.Kind || (a.Kind == b.Kind && a.ID < b.ID)
	})
	if m.SourceCounts == nil {
		m.SourceCounts = []SourceCount{}
	}
	sort.SliceStable(m.SourceCounts, func(i, j int) bool {
		a, b := m.SourceCounts[i], m.SourceCounts[j]
		return a.Kind < b.Kind || (a.Kind == b.Kind && a.ID < b.ID)
	})
	m.Compat = sortedStrings(m.Compat)
	if m.Files == nil {
		m.Files = []File{}
	}
	sort.SliceStable(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
}

// sortedStrings returns a sorted copy of s, never nil.
func sortedStrings(s []string) []string {
	out := append([]string{}, s...)
	sort.Strings(out)
	return out
}

// Validate checks the manifest against the v1 rules that the generated JSON
// schema cannot express alone: marker and version, RFC 3339 created_at,
// lowercase hex SHA-256 values, decimal table hashes, non-negative counts and
// safe, unique member paths (case-insensitively unique, so two members never
// land on one file of a case-insensitive filesystem).
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
	for _, t := range m.DB.Tables {
		if t.Rows < 0 || !hashRe.MatchString(t.Hash) {
			return fmt.Errorf("table %s.%s has an invalid row count or hash", t.Schema, t.Name)
		}
	}
	for _, o := range m.Storage.Objects {
		if o.Bytes < 0 || !sha256Re.MatchString(o.SHA256) {
			return fmt.Errorf("object %s/%s has an invalid size or sha256", o.Bucket, o.Key)
		}
	}
	seen := make(map[string]string, len(m.Files))
	for _, f := range m.Files {
		if err := CheckMember(f.Path); err != nil {
			return fmt.Errorf("file %q: %w", f.Path, err)
		}
		if f.Path == ManifestName {
			return fmt.Errorf("file %q: the manifest does not list itself", f.Path)
		}
		if f.Bytes < 0 || !sha256Re.MatchString(f.SHA256) {
			return fmt.Errorf("file %q has an invalid size or sha256", f.Path)
		}
		key := strings.ToLower(f.Path)
		if prev, dup := seen[key]; dup {
			return fmt.Errorf("file %q repeats member %q: %w", f.Path, prev, ErrDuplicate)
		}
		seen[key] = f.Path
	}
	for _, f := range m.Files {
		rest := f.Path
		for {
			i := strings.LastIndexByte(rest, '/')
			if i < 0 {
				break
			}
			rest = rest[:i]
			if other, clash := seen[strings.ToLower(rest)]; clash {
				return fmt.Errorf("file %q sits under member %q, which is a file", f.Path, other)
			}
		}
	}
	return nil
}
