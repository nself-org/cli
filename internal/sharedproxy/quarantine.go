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
// marked by this project) move; every file operation runs inside one os.Root
// opened on the owner dir, so a path component swapped for a symlink between
// the check and the use cannot lead outside the owner; each component is
// also Lstat-checked and a symlink or non-regular file is refused. The
// quarantine tree is mode 0700 (re-applied to directories that already
// exist) and quarantined files are 0600; Restore puts back 0644, or 0600 for
// private keys.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// quarantine file modes: the tree holds copies of live nginx files, so it is
// private; Restore returns the modes the build writes.

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

// ownerRel returns abs (a path under ownerDir) relative to ownerDir.
func ownerRel(ownerDir, abs string) (string, error) {
	rel, err := filepath.Rel(ownerDir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("quarantine path %s is outside %s", abs, ownerDir)
	}
	return rel, nil
}

// restoreMode is the mode Restore gives a file: private keys and the
// certificate marker stay 0600, the rest is what the build writes (0644).
func restoreMode(rel string) os.FileMode {
	b := filepath.Base(rel)
	if b == "privkey.pem" || b == CertMarker || strings.HasSuffix(b, ".key") {
		return 0o600
	}
	return 0o644
}

// quarantine moves the files of reg into the quarantine tree and returns the
// owner-relative paths it moved. Callers hold the owner lock (Quarantine).
func quarantine(ownerDir string, set Set, reg Registration) ([]string, error) {
	root, err := os.OpenRoot(ownerDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	baseRel, err := ownerRel(ownerDir, quarantineBase(ownerDir, reg.Project))
	if err != nil {
		return nil, err
	}
	sharedRel, err := ownerRel(ownerDir, DirOf(ownerDir))
	if err != nil {
		return nil, err
	}
	var moved []string
	for _, rel := range reg.Files {
		if !set.Removable(ownerDir, reg.Project, rel) {
			continue
		}
		src := filepath.FromSlash(rel)
		fi, err := root.Lstat(src)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return moved, err
		}
		if !fi.Mode().IsRegular() {
			return moved, fmt.Errorf("quarantine %s: not a regular file", rel)
		}
		dst := filepath.Join(baseRel, src)
		if err := mkdirPrivate(root, filepath.Dir(dst), sharedRel); err != nil {
			return moved, err
		}
		if err := root.Rename(src, dst); err != nil {
			return moved, fmt.Errorf("quarantine %s: %w", rel, err)
		}
		if err := root.Chmod(dst, 0o600); err != nil {
			return moved, fmt.Errorf("quarantine %s: %w", rel, err)
		}
		moved = append(moved, rel)
	}
	sort.Strings(moved)
	return moved, nil
}

// restore moves quarantined files of reg back (Restore is the locked entry). A path a newer build already
// wrote again stays as built and the stale quarantined copy is deleted. A
// quarantine tree that passes through a symlink is refused. It returns the
// paths put back.
func restore(ownerDir string, reg Registration) ([]string, error) {
	base := quarantineBase(ownerDir, reg.Project)
	if _, err := os.Lstat(base); err != nil {
		return nil, nil // nothing quarantined
	}
	root, err := os.OpenRoot(ownerDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	baseRel, err := ownerRel(ownerDir, base)
	if err != nil {
		return nil, err
	}
	if err := plainDirs(root, baseRel); err != nil {
		return nil, err
	}
	var back []string
	for _, rel := range reg.Files {
		if !ValidFile(rel) {
			continue
		}
		own := filepath.FromSlash(rel)
		q := filepath.Join(baseRel, own)
		if err := plainDirs(root, filepath.Dir(q)); err != nil {
			continue // a quarantine subdirectory that is a symlink: leave it
		}
		fi, err := root.Lstat(q)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if _, err := root.Lstat(own); err == nil {
			_ = root.Remove(q) // rebuilt since: the new file wins
			continue
		}
		if hasSymlink(ownerDir, rel) {
			continue
		}
		if err := root.MkdirAll(filepath.Dir(own), 0o700); err != nil {
			return back, err
		}
		if err := root.Rename(q, own); err != nil {
			return back, fmt.Errorf("restore %s: %w", rel, err)
		}
		if err := root.Chmod(own, restoreMode(rel)); err != nil {
			return back, fmt.Errorf("restore %s: %w", rel, err)
		}
		back = append(back, rel)
	}
	pruneEmpty(base)
	sort.Strings(back)
	return back, nil
}

// plainDirs checks that every component of rel (relative to root) that
// exists is a real directory: a symlink or a file on the way is an error.
func plainDirs(root *os.Root, rel string) error {
	cur := ""
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		fi, err := root.Lstat(cur)
		switch {
		case err != nil && os.IsNotExist(err):
			return nil
		case err != nil:
			return err
		case fi.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("quarantine: %s is a symlink", cur)
		case !fi.IsDir():
			return fmt.Errorf("quarantine: %s is not a directory", cur)
		}
	}
	return nil
}

// mkdirPrivate makes dir (relative to root) a chain of real directories,
// creating the missing ones with mode 0700 and setting 0700 on every one
// below shared (the owner's nginx/shared directory, which must already be a
// real directory and keeps its mode). A symlink or file on the way is refused.
func mkdirPrivate(root *os.Root, dir, shared string) error {
	cur := ""
	for _, part := range strings.Split(dir, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		private := cur != shared && strings.HasPrefix(cur, shared+string(filepath.Separator))
		fi, err := root.Lstat(cur)
		switch {
		case err == nil && fi.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("quarantine: %s is a symlink", cur)
		case err == nil && !fi.IsDir():
			return fmt.Errorf("quarantine: %s is not a directory", cur)
		case err == nil:
			if private && fi.Mode().Perm() != 0o700 {
				if err := root.Chmod(cur, 0o700); err != nil {
					return err
				}
			}
			continue
		case !os.IsNotExist(err):
			return err
		}
		if err := root.Mkdir(cur, 0o700); err != nil {
			return err
		}
		if private {
			if err := root.Chmod(cur, 0o700); err != nil {
				return err
			}
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
