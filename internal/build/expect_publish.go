package build

// Purpose: publish a completely verified render and restore prior artifacts on
// an ordinary publication error. Inputs: the staged sink. Outputs: disk files.
// Constraints: multi-file publication is not crash atomic; host effects are
// outside this transaction. Rollback errors are returned alongside the cause.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type artifactBefore struct {
	path       string
	info       fs.FileInfo
	data       string // secure staging file containing prior regular-file bytes
	link       string
	targetMode *fs.FileMode
}

type artifactPublisher struct {
	sink    *expectSink
	stage   string
	roots   map[string]*os.Root
	before  []artifactBefore
	created []string
	files   map[string]string
}

func (e *expectSink) commit(validate ...func() error) error {
	stage, err := os.MkdirTemp("", "nself-artifacts-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	p := &artifactPublisher{sink: e, stage: stage, roots: map[string]*os.Root{}, files: map[string]string{}}
	defer p.close()
	if err := p.stageFiles(); err != nil {
		return err
	}
	if err := e.verify(); err != nil {
		return err
	}
	for k := range e.dirs {
		if strings.HasPrefix(k, ".nself/backups/") {
			if err := checkBackupRoot(e.root); err != nil {
				return err
			}
			break
		}
	}
	if err := p.publish(); err != nil {
		return errors.Join(err, p.rollback())
	}
	for _, check := range validate {
		if err := check(); err != nil {
			return errors.Join(err, p.rollback())
		}
	}
	if err := e.verifyPublished(); err != nil {
		return errors.Join(err, p.rollback())
	}
	return nil
}

func (p *artifactPublisher) stageFiles() error {
	for _, k := range sortedArtifactKeys(p.sink.keyr.files) {
		file := filepath.Join(p.stage, fmt.Sprintf("render-%d", len(p.files)))
		if err := os.WriteFile(file, p.sink.keyr.files[k].Data, 0600); err != nil {
			return err
		}
		p.files[k] = file
	}
	return nil
}

func (p *artifactPublisher) close() {
	for _, root := range p.roots {
		_ = root.Close()
	}
}

// target confines project writes (including backups) with os.Root. External
// plugin and fronting paths retain their original absolute destinations.
func (p *artifactPublisher) target(path string) (*os.Root, string, error) {
	abs, err := filepath.Abs(p.sink.abs(path))
	if err != nil {
		return nil, "", err
	}
	base := filepath.VolumeName(abs) + string(filepath.Separator)
	rel, inside := under(p.sink.root, abs)
	if inside {
		base = p.sink.root
	} else {
		rel, err = filepath.Rel(base, abs)
	}
	if err != nil {
		return nil, "", err
	}
	root := p.roots[base]
	if root == nil {
		root, err = os.OpenRoot(base)
		if err != nil {
			return nil, "", err
		}
		p.roots[base] = root
	}
	return root, filepath.FromSlash(rel), nil
}

func (p *artifactPublisher) publish() error {
	for _, k := range sortedArtifactKeys(p.sink.dirs) {
		if err := p.mkdir(p.sink.abs(k), p.sink.dirs[k]); err != nil {
			return err
		}
	}
	for _, k := range sortedArtifactKeys(p.sink.keyr.files) {
		file := p.sink.keyr.files[k]
		dest, err := p.destination(k)
		if err != nil {
			return err
		}
		if err := p.mkdir(filepath.Dir(p.sink.abs(k)), 0755); err != nil {
			return err
		}
		if err := p.remember(dest); err != nil {
			return err
		}
		if !p.sink.atomic[k] {
			if err := p.checkWritable(k, dest); err != nil {
				return err
			}
		}
		data, err := os.ReadFile(p.files[k])
		if err != nil {
			return err
		}
		if err := p.write(dest, data, file.Perm); err != nil {
			return err
		}
	}
	removed := sortedArtifactKeys(p.sink.keyr.removed)
	// Children must be removed before an emptied snapshot directory.
	sort.Sort(sort.Reverse(sort.StringSlice(removed)))
	for _, k := range removed {
		if err := p.remember(k); err != nil {
			return err
		}
		root, rel, err := p.target(k)
		if err != nil {
			return err
		}
		if err := root.Remove(rel); err != nil {
			return err
		}
	}
	for _, k := range sortedArtifactKeys(p.sink.keyr.modes) {
		dest, err := p.followDestination(k)
		if err != nil {
			return err
		}
		if err := p.remember(dest); err != nil {
			return err
		}
		root, rel, err := p.target(dest)
		if err != nil {
			return err
		}
		if err := root.Chmod(rel, p.sink.keyr.modes[k]); err != nil {
			return err
		}
	}
	return nil
}

func sortedArtifactKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (p *artifactPublisher) mkdir(path string, perm fs.FileMode) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", path)
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := p.mkdir(filepath.Dir(path), perm); err != nil {
		return err
	}
	root, rel, err := p.target(path)
	if err != nil {
		return err
	}
	if err := root.Mkdir(rel, perm); err != nil {
		return err
	}
	p.created = append(p.created, path)
	return nil
}

func (p *artifactPublisher) write(path string, data []byte, mode fs.FileMode) error {
	root, rel, err := p.target(path)
	if err != nil {
		return err
	}
	tmp := rel + "." + filepath.Base(p.stage)
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(tmp) }()
	_, err = f.Write(data)
	if err == nil {
		err = f.Chmod(mode)
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return root.Rename(tmp, rel)
}
