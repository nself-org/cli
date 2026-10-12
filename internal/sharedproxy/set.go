package sharedproxy

// set.go — reading every registration of an owner dir, the ownership rules
// built on it, and the disk writer and Unregister (contract:cli.shared-proxy v1).
//
// Purpose: no project deletes a file another registration lists; `nself
// uninstall` in a fronted project calls Unregister.
// Inputs: an owner dir and its nginx/shared/ directory.
// Outputs: Set (all registrations), Removable, Write, Unregister.
// Constraints: Load fails closed on any file that does not validate.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Set is every registration found in one nginx/shared/ directory.
type Set struct {
	Owner    *Registration  // _owner.json, nil when absent
	Projects []Registration // sorted by project
}

// Load reads dir (`<owner>/nginx/shared`). A missing dir is an empty Set. A
// file that does not validate, or a second registration of one project, is an
// error naming the file: a fail-closed read, never a skipped entry.
func Load(dir, ownerDir string) (Set, error) {
	var set Set
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return set, nil
	}
	if err != nil {
		return set, err
	}
	seen := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return set, err
		}
		r, err := Parse(name, data, ownerDir)
		if err != nil {
			return set, err
		}
		if name == OwnerFile {
			set.Owner = &r
			continue
		}
		if prev, dup := seen[r.Project]; dup {
			return set, fmt.Errorf("%s and %s both register project %q", prev, name, r.Project)
		}
		seen[r.Project] = name
		set.Projects = append(set.Projects, r)
	}
	sort.Slice(set.Projects, func(i, j int) bool { return set.Projects[i].Project < set.Projects[j].Project })
	return set, nil
}

// RegisteredFiles returns every path (relative to the owner dir) any project
// registration lists, as a set. The owner's own file lists none.
func (s Set) RegisteredFiles() map[string]bool {
	out := map[string]bool{}
	for _, p := range s.Projects {
		for _, f := range p.Files {
			out[f] = true
		}
	}
	return out
}

// Others returns the project registrations other than project's own.
func (s Set) Others(project string) []Registration {
	var out []Registration
	for _, p := range s.Projects {
		if p.Project != project {
			out = append(out, p)
		}
	}
	return out
}

// Find returns the registration of project, or nil.
func (s Set) Find(project string) *Registration {
	for i := range s.Projects {
		if s.Projects[i].Project == project {
			return &s.Projects[i]
		}
	}
	return nil
}

// hasSymlink reports whether any component of rel below root is a symlink.
func hasSymlink(root, rel string) bool {
	p := root
	for _, part := range strings.Split(rel, "/") {
		p = filepath.Join(p, part)
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

// Removable reports whether project may delete rel (owner-relative): a valid
// registered path that no other project lists and that does not pass through a
// symlink (an ACME-managed pair is a symlink, and is never removed here).
func (s Set) Removable(ownerDir, project, rel string) bool {
	if !ValidFile(rel) || hasSymlink(ownerDir, rel) {
		return false
	}
	for _, o := range s.Others(project) {
		for _, f := range o.Files {
			if f == rel {
				return false
			}
		}
	}
	return true
}

// Write writes r as `<project>.json` (or `_owner.json` for name) into dir,
// creating dir, via a temp file and rename.
func Write(dir, name string, r Registration) error {
	data, err := Marshal(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".reg-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

// Unregister removes every file project's registration lists (only those
// Removable allows), empty certificate directories left behind, and then the
// registration itself. It returns the owner-relative files it removed. A
// missing registration is not an error. `nself uninstall` in a fronted project
// calls this.
func Unregister(ownerDir, project string) ([]string, error) {
	dir := DirOf(ownerDir)
	set, err := Load(dir, ownerDir)
	if err != nil {
		return nil, err
	}
	reg := set.Find(project)
	if reg == nil {
		return nil, nil
	}
	var removed []string
	for _, rel := range reg.Files {
		if !set.Removable(ownerDir, project, rel) {
			continue
		}
		path := filepath.Join(ownerDir, filepath.FromSlash(rel))
		if err := os.Remove(path); err == nil {
			removed = append(removed, rel)
		} else if !os.IsNotExist(err) {
			return removed, err
		}
		if strings.HasPrefix(rel, "ssl/certificates/") {
			_ = os.Remove(filepath.Dir(path)) // fails while the directory is not empty
		}
	}
	if err := os.Remove(filepath.Join(dir, FileName(project))); err != nil && !os.IsNotExist(err) {
		return removed, err
	}
	return removed, nil
}
