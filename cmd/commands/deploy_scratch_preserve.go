package commands

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// preserveScratchNginx saves explicitly included generated files before the
// build prunes stale sites. Restore only missing entries, so fresh build output
// wins. Both reads and restores stay inside the scratch project.
func preserveScratchNginx(scratch string, policy *deploy.NginxPolicy) (func() error, error) {
	type savedFile struct {
		name string
		data []byte
		mode fs.FileMode
	}
	var saved []savedFile
	nginxDir, err := nginxtopo.ServedNginxDir(scratch, "")
	if err != nil {
		return nil, err
	}
	err = filepath.WalkDir(nginxDir, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		sub, _ := filepath.Rel(nginxDir, p)
		if !policy.Allows(filepath.ToSlash(sub)) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !build.IsGeneratedNginxConf(data) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(scratch, p)
		saved = append(saved, savedFile{rel, data, info.Mode().Perm()})
		return nil
	})
	return func() error {
		root, err := os.OpenRoot(scratch)
		if err != nil {
			return err
		}
		defer func() { _ = root.Close() }()
		for _, f := range saved {
			if _, err := root.Lstat(f.name); err == nil {
				continue
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err := root.MkdirAll(filepath.Dir(f.name), 0755); err != nil {
				return err
			}
			out, err := root.OpenFile(f.name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, f.mode)
			if err != nil {
				return err
			}
			_, writeErr := out.Write(f.data)
			closeErr := out.Close()
			if err := errors.Join(writeErr, closeErr); err != nil {
				return err
			}
		}
		return nil
	}, err
}
