package sharedproxy

// unregister.go — Unregister's disk work, done inside an os.Root on the owner dir.
//
// Purpose: remove a leaving project's files without a check-then-delete race.
// Removable decides by absolute path; a directory swapped for a symlink after
// that check must not redirect the delete. Every removal here goes through
// root.Remove on an owner-relative path (the Root refuses a path that escapes
// the owner dir) after a plainDirs check on the parent chain.
// Inputs: the owner dir and the project name.
// Outputs: the owner-relative files removed.
// Constraints: a symlink on the way is an E506 error, never followed; a file
// already gone is not an error.

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// beforeRemove runs after Removable approved rel and before it is deleted; a
// variable so a test can swap a directory for a symlink in that window.
var beforeRemove = func(rel string) {}

// removeIn deletes rel (owner-relative, slash form) through root. The parent
// chain must be plain directories. It reports whether the file existed.
func removeIn(root *os.Root, rel string) (bool, error) {
	native := filepath.FromSlash(rel)
	if err := plainDirs(root, filepath.Dir(native)); err != nil {
		return false, e506("refusing to remove "+rel, err, "")
	}
	err := root.Remove(native)
	switch {
	case err == nil:
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	}
	return false, e506("cannot remove "+rel, err, "")
}

// unregister (locked entry: Unregister) removes every file project's registration lists (only those
// Removable allows), empty certificate directories left behind, and then the
// registration itself. It returns the owner-relative files it removed. A
// missing registration is not an error. `nself uninstall` in a fronted project
// calls this.
func unregister(ownerDir, project string) ([]string, error) {
	dir := DirOf(ownerDir)
	set, err := Load(dir, ownerDir)
	if err != nil {
		return nil, err
	}
	reg := set.Find(project)
	if reg == nil {
		return nil, nil
	}
	root, err := os.OpenRoot(ownerDir)
	if err != nil {
		return nil, e506("cannot open the shared proxy owner dir "+ownerDir, err, "")
	}
	defer func() { _ = root.Close() }()
	var removed []string
	certDirs := map[string]bool{}
	for _, rel := range reg.Files {
		if !set.Removable(ownerDir, project, rel) {
			continue
		}
		beforeRemove(rel)
		gone, err := removeIn(root, rel)
		if err != nil {
			return removed, err
		}
		if gone {
			removed = append(removed, rel)
		}
		if strings.HasPrefix(rel, "ssl/certificates/") {
			certDirs[path.Dir(rel)] = true
		}
	}
	for d := range certDirs { // the marker goes last, then the directory if empty
		if _, err := removeIn(root, d+"/"+CertMarker); err != nil {
			return removed, err
		}
		// Best effort by design: the directory stays when it still holds files.
		_ = root.Remove(filepath.FromSlash(d))
	}
	regRel, err := ownerRel(ownerDir, filepath.Join(dir, FileName(project)))
	if err != nil {
		return removed, e506("cannot remove the registration of "+project, err, "")
	}
	if _, err := removeIn(root, filepath.ToSlash(regRel)); err != nil {
		return removed, err
	}
	return removed, nil
}
