package sharedproxy

// locate.go — find a project's place in a shared host proxy from its directory.
//
// Purpose: the runtime half (P7-ADOPT-21) must know, from a project directory
// alone, whether it owns a shared nginx or is registered with one. The compose
// pre-down hook has only the directory, so nothing here reads the environment.
// Inputs: a project directory.
// Outputs: Site (role, owner dir, the project's registration, the owner's Set).
// Constraints: the fronting layout is the one nginxtopo trusts: the owner is
// the direct parent directory. A project is registered only while the owner's
// `_owner.json` exists. No docker call.

import (
	"os"
	"path/filepath"
)

// Role is what a project is to a shared host proxy.
type Role int

// The roles Locate reports.
const (
	RoleNone    Role = iota // not part of a shared proxy
	RoleOwner               // owns nginx/shared/_owner.json
	RoleFronted             // registered under its parent directory's proxy
)

// Site is the result of Locate.
type Site struct {
	Role     Role
	OwnerDir string        // the owner's project directory (the project itself for RoleOwner)
	Reg      *Registration // the project's own registration; nil for RoleOwner and RoleNone
	Set      Set           // every registration under OwnerDir
}

// Locate reports the role of the project at workdir. An unreadable or invalid
// registration directory is returned as an error with RoleNone.
func Locate(workdir string) (Site, error) {
	workdir = filepath.Clean(workdir)
	set, err := Load(DirOf(workdir), workdir)
	if err != nil {
		return Site{}, err
	}
	if set.Owner != nil {
		return Site{Role: RoleOwner, OwnerDir: workdir, Set: set}, nil
	}
	parent := filepath.Dir(workdir)
	if parent == workdir {
		return Site{}, nil
	}
	set, err = Load(DirOf(parent), parent)
	if err != nil {
		return Site{}, err
	}
	if set.Owner == nil {
		return Site{}, nil // the owner dropped shared mode, or never had it
	}
	for i := range set.Projects {
		if sameDir(set.Projects[i].ProjectDir, workdir) {
			return Site{Role: RoleFronted, OwnerDir: parent, Reg: &set.Projects[i], Set: set}, nil
		}
	}
	return Site{}, nil
}

// sameDir reports whether a and b are the same directory, by cleaned path or,
// failing that, by file identity (a symlinked ~/Sites).
func sameDir(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ia, err1 := os.Stat(a)
	ib, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(ia, ib)
}
