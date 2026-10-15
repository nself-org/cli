package build

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func (p *artifactPublisher) remember(path string) error {
	root, rel, err := p.target(path)
	if err != nil {
		return err
	}
	before := artifactBefore{path: path}
	before.info, err = root.Lstat(rel)
	if errors.Is(err, fs.ErrNotExist) {
		p.before = append(p.before, before)
		return nil
	}
	if err != nil {
		return err
	}
	switch {
	case before.info.Mode().IsRegular():
		data, err := root.ReadFile(rel)
		if err != nil {
			return err
		}
		before.data = filepath.Join(p.stage, fmt.Sprintf("before-%d", len(p.before)))
		if err := os.WriteFile(before.data, data, 0600); err != nil {
			return err
		}
	case before.info.Mode()&fs.ModeSymlink != 0:
		before.link, err = root.Readlink(rel)
		if err != nil {
			return err
		}
		if target, statErr := root.Stat(rel); statErr == nil {
			mode := target.Mode().Perm()
			before.targetMode = &mode
		}
	case !before.info.IsDir():
		return fmt.Errorf("refusing to replace special file %s", path)
	}
	p.before = append(p.before, before)
	return nil
}

func (p *artifactPublisher) rollback() error {
	var result error
	for i := len(p.before) - 1; i >= 0; i-- {
		if err := p.restore(p.before[i]); err != nil {
			result = errors.Join(result, fmt.Errorf("restoring %s: %w", p.before[i].path, err))
		}
	}
	for i := len(p.created) - 1; i >= 0; i-- {
		root, rel, err := p.target(p.created[i])
		if err == nil {
			err = root.Remove(rel)
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}
	return result
}

func (p *artifactPublisher) restore(before artifactBefore) error {
	root, rel, err := p.target(before.path)
	if err != nil {
		return err
	}
	if before.info == nil {
		err := root.Remove(rel)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if before.info.IsDir() {
		if err := root.Mkdir(rel, before.info.Mode().Perm()); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		return root.Chmod(rel, before.info.Mode().Perm())
	}
	if before.link != "" {
		if err := root.Remove(rel); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := root.Symlink(before.link, rel); err != nil {
			return err
		}
		if before.targetMode != nil {
			return root.Chmod(rel, *before.targetMode)
		}
		return nil
	}
	data, err := os.ReadFile(before.data)
	if err != nil {
		return err
	}
	return p.write(before.path, data, before.info.Mode().Perm())
}
