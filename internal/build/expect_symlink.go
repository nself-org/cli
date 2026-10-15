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

// checkWritable preserves os.WriteFile's access check before atomic promotion:
// replacing a directory entry alone would bypass a read-only existing file.
func (p *artifactPublisher) checkWritable(key, dest string) error {
	root, rel, err := p.target(dest)
	if err != nil {
		return err
	}
	if mode, ok := p.sink.writeModes[key]; ok {
		if err := root.Chmod(rel, mode); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	f, err := root.OpenFile(rel, os.O_WRONLY, 0)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return f.Close()
}
