package sharedproxy

// quarantine.go — move a project's files out of the owner's nginx so a broken
// conf cannot keep the owner from reloading.
//
// Purpose: review F4 and R3 of EPIC ADOPT D2. `nginx -t` names the file that
// breaks the combined configuration; the project that registered it is
// quarantined (its files move to <owner>/nginx/shared/quarantine/<project>/,
// outside every include path) so the owner keeps serving. The next successful
// attach restores them.
// Inputs: the owner dir, the Set, a registration; for Implicated the nginx
// output text.
// Outputs: moved/restored owner-relative paths; the registrations the text
// implicates.
// Constraints: only files the project may remove (Set.Removable: valid path,
// no symlink on the way, not listed by another project, certificate dir
// marked by this project) move; the quarantine tree is mode 0700 and each
// path component is Lstat-checked; a symlink or non-regular file is refused.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// QuarantineDir is the directory name under nginx/shared/ that holds
// quarantined files.
const QuarantineDir = "quarantine"

func quarantineBase(ownerDir, project string) string {
	return filepath.Join(DirOf(ownerDir), QuarantineDir, project)
}

// Implicated returns the registered projects (never the owner) that own a file
// the nginx output names. A site file matches by its base name (the container
// path differs from the host path), a certificate file by
// `certificates/<dir>/<file>`. Sorted by project.
func Implicated(set Set, text string) []Registration {
	var out []Registration
	for _, r := range set.Projects {
		for _, rel := range r.Files {
			if mentions(text, rel) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

func mentions(text, rel string) bool {
	needle := filepath.Base(rel)
	if strings.HasPrefix(rel, "ssl/") {
		needle = strings.TrimPrefix(rel, "ssl/")
	}
	re := regexp.MustCompile(`(^|[/\s"'])` + regexp.QuoteMeta(needle) + `($|[:\s"'])`)
	return re.MatchString(text)
}

// Quarantine moves the files of reg into the quarantine tree and returns the
// owner-relative paths it moved.
func Quarantine(ownerDir string, set Set, reg Registration) ([]string, error) {
	base := quarantineBase(ownerDir, reg.Project)
	var moved []string
	for _, rel := range reg.Files {
		if !set.Removable(ownerDir, reg.Project, rel) {
			continue
		}
		src := filepath.Join(ownerDir, filepath.FromSlash(rel))
		fi, err := os.Lstat(src)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return moved, err
		}
		if !fi.Mode().IsRegular() {
			return moved, fmt.Errorf("quarantine %s: not a regular file", rel)
		}
		dst := filepath.Join(base, filepath.FromSlash(rel))
		if err := mkdirPrivate(filepath.Dir(dst), DirOf(ownerDir)); err != nil {
			return moved, err
		}
		if err := os.Rename(src, dst); err != nil {
			return moved, fmt.Errorf("quarantine %s: %w", rel, err)
		}
		moved = append(moved, rel)
	}
	sort.Strings(moved)
	return moved, nil
}

// Restore moves quarantined files of reg back. A path a newer build already
// wrote again stays as built and the stale quarantined copy is deleted. It
// returns the paths put back.
func Restore(ownerDir string, reg Registration) ([]string, error) {
	base := quarantineBase(ownerDir, reg.Project)
	if _, err := os.Lstat(base); err != nil {
		return nil, nil // nothing quarantined
	}
	var back []string
	for _, rel := range reg.Files {
		if !ValidFile(rel) {
			continue
		}
		q := filepath.Join(base, filepath.FromSlash(rel))
		fi, err := os.Lstat(q)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		dst := filepath.Join(ownerDir, filepath.FromSlash(rel))
		if _, err := os.Lstat(dst); err == nil {
			_ = os.Remove(q) // rebuilt since: the new file wins
			continue
		}
		if hasSymlink(ownerDir, rel) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return back, err
		}
		if err := os.Rename(q, dst); err != nil {
			return back, fmt.Errorf("restore %s: %w", rel, err)
		}
		back = append(back, rel)
	}
	pruneEmpty(base)
	sort.Strings(back)
	return back, nil
}

// mkdirPrivate creates dir and every missing parent below stop with mode 0700,
// refusing a symlink or a non-directory on the way.
func mkdirPrivate(dir, stop string) error {
	rel, err := filepath.Rel(stop, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("quarantine path %s is outside %s", dir, stop)
	}
	cur := stop
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		switch {
		case err == nil && fi.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("quarantine: %s is a symlink", cur)
		case err == nil && !fi.IsDir():
			return fmt.Errorf("quarantine: %s is not a directory", cur)
		case err == nil:
			continue
		case !os.IsNotExist(err):
			return err
		}
		if err := os.Mkdir(cur, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(cur, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// pruneEmpty removes empty directories under and including root, deepest first.
func pruneEmpty(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i]) // fails harmlessly when not empty
	}
}
