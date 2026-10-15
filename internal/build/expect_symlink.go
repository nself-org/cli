package build

import (
	"io/fs"
	"os"
	"path/filepath"
)

// destination preserves WriteFile's existing symlink-follow behavior while
// WriteAtomic replaces the named entry. Backup entries stay root-confined.
func (p *artifactPublisher) destination(k string) (string, error) {
	if p.sink.atomic[k] || p.sink.backupPath(k) {
		return k, nil
	}
	return p.followDestination(k)
}

func (p *artifactPublisher) followDestination(k string) (string, error) {
	abs := p.sink.abs(k)
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		return k, nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return k, nil
	}
	return filepath.EvalSymlinks(abs)
}
