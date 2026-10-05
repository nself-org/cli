package portable

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// readManifest loads, version-checks, decodes and validates manifest.json.
//
// Decoding is strict for a "1" (or "1.0") bundle: an unknown field is refused.
// A later minor ("1.n", n > 0) may add fields, which are ignored. In every
// version a duplicate key, or two keys that differ only in case, or a key that
// matches a field only case-insensitively, is refused, because Go and other
// JSON readers resolve those differently.
func readManifest(root string, lim Limits) (Manifest, error) {
	raw, err := readManifestBytes(root, lim)
	if err != nil {
		return Manifest{}, err
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
	if err := checkKeys(raw); err != nil {
		return Manifest{}, integrityErr(ErrManifest, "%s has ambiguous keys: %v", ManifestName, err)
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	if !additiveMinor(head.SchemaVersion) {
		dec.DisallowUnknownFields()
	}
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

// additiveMinor reports whether v is "1.n" with some component after the major
// not zero: a minor release, which may add fields.
func additiveMinor(v string) bool {
	return strings.Trim(strings.TrimPrefix(v, "1"), ".0") != ""
}

// readManifestBytes reads manifest.json: a regular, singly linked file, opened
// without following a symlink, within the size limit.
func readManifestBytes(root string, lim Limits) ([]byte, error) {
	p := filepath.Join(root, ManifestName)
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, formatErr(ErrManifest, "%s not found: not a portable bundle", ManifestName)
	}
	if !fi.Mode().IsRegular() {
		return nil, integrityErr(ErrLink, "%s is not a plain regular file", ManifestName)
	}
	if fi.Size() > lim.MaxManifestBytes {
		return nil, integrityErr(ErrSize, "%s is %d bytes, limit is %d", ManifestName, fi.Size(), lim.MaxManifestBytes)
	}
	fh, err := os.OpenFile(p, openFlags, 0)
	if err != nil {
		return nil, formatErr(ErrManifest, "%s cannot be opened", ManifestName)
	}
	defer func() { _ = fh.Close() }()
	_, nlink, st, err := snapshotOf(fh)
	if err != nil || !os.SameFile(fi, st) {
		return nil, integrityErr(ErrLink, "%s changed while it was opened", ManifestName)
	}
	if nlink > 1 {
		return nil, integrityErr(ErrLink, "%s is not a plain regular file", ManifestName)
	}
	raw, err := io.ReadAll(io.LimitReader(fh, lim.MaxManifestBytes+1))
	if err != nil || int64(len(raw)) > lim.MaxManifestBytes {
		return nil, integrityErr(ErrSize, "%s cannot be read within %d bytes", ManifestName, lim.MaxManifestBytes)
	}
	return raw, nil
}

// truncate shortens an attacker-controlled string for an error message.
func truncate(s string) string {
	const max = 40
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
