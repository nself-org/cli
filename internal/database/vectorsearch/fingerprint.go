package vectorsearch

// fingerprint.go closes the gap between the drift check and the mirror write
// (D-0369): track hashes the on-disk metadata before it compares disk with
// live Hasura, and the mirror writes only while the files still hash the same.
// A concurrent export or hand edit in between leaves disk untouched.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrDiskChanged means the on-disk metadata changed after the drift check
// proved it equal to live Hasura; nothing was written.
var ErrDiskChanged = errors.New("on-disk metadata changed after the drift check")

// DiskFingerprint hashes the project's on-disk metadata: every regular file
// under hasura/metadata (path and bytes, in walk order, which is lexical),
// else hasura/metadata.json. It returns "" when neither exists.
func DiskFingerprint(projectDir string) (string, error) {
	_, jsonFile, yamlDir := metadataPaths(projectDir)
	h := sha256.New()
	if st, err := os.Stat(yamlDir); err == nil && st.IsDir() {
		err := filepath.WalkDir(yamlDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() {
				return err
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(yamlDir, path)
			_, _ = fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(body))
			_, _ = h.Write(body)
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("hash %s: %w", yamlDir, err)
		}
		return "dir:" + hex.EncodeToString(h.Sum(nil)), nil
	}
	body, err := os.ReadFile(jsonFile)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", jsonFile, err)
	}
	_, _ = h.Write(body)
	return "json:" + hex.EncodeToString(h.Sum(nil)), nil
}

// MirrorFunctionIfUnchanged is MirrorFunction that writes only while the
// on-disk metadata still has fingerprint want (from DiskFingerprint taken
// before the drift check). The files are hashed again after the edited file
// is read and before it is written; a different hash is ErrDiskChanged and
// nothing is written.
func MirrorFunctionIfUnchanged(projectDir, schema, name string, roles []string, want string) (string, bool, error) {
	return mirror(projectDir, schema, name, roles, func() error {
		got, err := DiskFingerprint(projectDir)
		if err != nil {
			return err
		}
		if got != want {
			return ErrDiskChanged
		}
		return nil
	})
}
