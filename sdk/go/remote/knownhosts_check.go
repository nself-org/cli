// Purpose: safety checks and the lock for the pinned known_hosts file.
// Inputs:  the pin file path, the file's lines, the parent directory.
// Outputs: errors naming the path and 1-based line numbers; a release func.
// Constraints: never echoes a key blob. The parent dir may be a symlink (the
//              target is checked); the pin file may not (rename would replace
//              the link). Mode checks are skipped on windows.
//              Nothing here chmods a user directory. Lock is the sidecar
//              "<Path>.lock" (0600, never truncated, left in place) because
//              write() renames over the pin file and so changes its inode.

package remote

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// foreignChars in a host field mean ssh would match more than one exact alias.
const foreignChars = "*?!,[]|"

// foreignLines returns one "line N: reason" per line ssh would honour but a
// pinned file cannot represent: @markers, wildcards, negation, lists,
// [host]:port and hashed hosts. Blank and # comment lines are fine.
func foreignLines(raw string) []string {
	var out []string
	for i, l := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || t[0] == '#' {
			continue
		}
		f := strings.Fields(t)[0]
		switch {
		case f[0] == '@':
			out = append(out, fmt.Sprintf("line %d: %s marker is not supported in a pinned file; remove it by hand", i+1, f))
		case strings.ContainsAny(f, foreignChars):
			out = append(out, fmt.Sprintf("line %d: host field uses wildcard, negation, list, [host]:port or hashed syntax, which a pinned file does not support; remove it by hand", i+1))
		}
	}
	return out
}

// checkDir refuses a pin directory that is not a directory or that group or
// world can write. A symlink is followed (home dot-dirs are often offloaded
// to another disk by symlink) and the target is checked. A missing dir
// returns the Stat error.
func checkDir(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("pinned host keys directory %s is not a directory (mode %v)", dir, fi.Mode())
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("pinned host keys directory %s has mode %04o: group/world writable; chmod it 0700", dir, fi.Mode().Perm())
	}
	return nil
}

// checkFile refuses an existing pin file that is not a regular file or that
// group or world can write. A missing file is fine.
func checkFile(path string) error {
	fi, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("pinned host keys file %s is not a regular file (mode %v)", path, fi.Mode())
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("pinned host keys file %s has mode %04o: group/world writable; chmod it 0600", path, fi.Mode().Perm())
	}
	return nil
}

func noUnlock() {}

// acquire takes the cross-process lock and returns its release func. Writers
// create the directory (0700) and the lock file and hold LOCK_EX. Readers take
// LOCK_SH only if the directory and lock file already exist; otherwise there
// is nothing to race with and nothing is created.
func (p PinnedHostKeys) acquire(write bool) (func(), error) {
	if p.Path == "" {
		return nil, fmt.Errorf("PinnedHostKeys.Path is empty")
	}
	dir := filepath.Dir(p.Path)
	flags := os.O_RDONLY
	if write {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		flags = os.O_CREATE | os.O_RDWR
	}
	if err := checkDir(dir); err != nil {
		if !write && os.IsNotExist(err) {
			return noUnlock, nil
		}
		return nil, err
	}
	f, err := os.OpenFile(p.Path+".lock", flags, 0o600)
	if err != nil {
		if !write && os.IsNotExist(err) {
			return noUnlock, nil
		}
		return nil, err
	}
	if err := lockFD(f, write); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { unlockFD(f); _ = f.Close() }, nil
}
