package agentsetup

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Purpose: the two disk writes shared by `nself mcp setup --write` and
// `nself init`. Inputs: the project dir. Outputs: the file path and whether
// bytes changed on disk. Constraints: only fixed names under dir; a symlink in
// the way is refused so nothing is written outside the project.

// WriteAgentsMD writes AGENTS.md when it is absent or an unedited generated
// file; any other file is left untouched (written false, no error).
func WriteAgentsMD(dir string, verbs []Verb) (string, bool, error) {
	p := filepath.Join(dir, FileName)
	old, exists, err := readRegular(p)
	if err != nil {
		return p, false, err
	}
	if !MayWrite(exists, string(old)) {
		return p, false, nil
	}
	w, err := writeIfChanged(p, old, []byte(Render(verbs)))
	return p, w, err
}

// WriteClient merges the nself server entry into the client's config file
// under dir, creating it when absent. Print-only clients are an error.
func WriteClient(dir, client string) (string, bool, error) {
	rel := Path(client)
	if rel == "" {
		return "", false, fmt.Errorf("%s is print-only: copy the printed config into the client yourself", client)
	}
	p := filepath.Join(dir, filepath.FromSlash(rel))
	old, _, err := readRegular(p)
	if err != nil {
		return p, false, err
	}
	merged, changed, err := Merge(old)
	if err != nil {
		return p, false, fmt.Errorf("%s: %w", rel, err)
	}
	if !changed {
		return p, false, nil
	}
	w, err := writeIfChanged(p, old, merged)
	return p, w, err
}

// readRegular reads p. A missing file is (nil, false, nil); a symlink or any
// non-regular file is an error.
func readRegular(p string) ([]byte, bool, error) {
	fi, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !fi.Mode().IsRegular() {
		return nil, true, fmt.Errorf("%s is not a regular file (symlinks are not followed)", p)
	}
	b, err := os.ReadFile(p)
	return b, true, err
}

func writeIfChanged(p string, old, next []byte) (bool, error) {
	if bytes.Equal(old, next) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".nself-agent-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(next); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return false, err
	}
	return true, os.Rename(tmp.Name(), p)
}
