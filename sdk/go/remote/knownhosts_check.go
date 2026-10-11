// Purpose: safety checks and the lock for the pinned known_hosts file.
// Inputs:  the pin file path, the file's lines, the parent directory.
// Outputs: errors naming the path and 1-based line numbers; a release func.
// Constraints: never echoes a key blob. The parent dir may be a symlink (the
//              target is checked); the pin file may not (rename would replace
//              the link). Mode and owner checks are skipped on windows.
//              Nothing here chmods a user directory. Lock is the sidecar
//              "<Path>.lock" (0600, never truncated, left in place, opened
//              O_NOFOLLOW|O_NONBLOCK, and must be a regular file
//              we own and nobody else can write) because write() renames over the pin file and so
//              changes its inode. Lines are split as ssh does: on ' ' and
//              '\t' only, after one trailing '\r' is dropped.

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

// splitLine strips one trailing '\r' and splits on ' ' and '\t' only (runs
// collapse), the way ssh ends the host field. strings.Fields would also split
// on \v, \f, U+0085 and U+00A0, which ssh does not.
func splitLine(l string) []string {
	return strings.FieldsFunc(strings.TrimSuffix(l, "\r"), func(r rune) bool { return r == ' ' || r == '\t' })
}

// hostFields returns the fields of a key line, or nil for a blank or comment line.
func hostFields(l string) []string {
	f := splitLine(l)
	if len(f) == 0 || f[0][0] == '#' {
		return nil
	}
	return f
}

// hasControl reports a byte < 0x20 other than tab, or 0x7f.
func hasControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if (s[i] < 0x20 && s[i] != '\t') || s[i] == 0x7f {
			return true
		}
	}
	return false
}

// hasHighByte reports a non-ASCII byte.
func hasHighByte(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return true
		}
	}
	return false
}

// foreignLines returns one "line N: reason" per line ssh would honour but a
// pinned file cannot represent: @markers, wildcards, negation, lists,
// [host]:port, hashed hosts, a line with fewer than 3 fields or a second field that is not a
// key type, and lines whose host field ssh would end at a
// different byte than Go does (control characters, non-ASCII). Blank and
// # comment lines are fine.
func foreignLines(raw string) []string {
	var out []string
	for i, l := range strings.Split(raw, "\n") {
		f := hostFields(l)
		if f == nil {
			continue
		}
		// if / else-if, not switch: a case expression has no cover block, so
		// the mutation tool could not measure a condition written there.
		if hasControl(strings.TrimSuffix(l, "\r")) {
			out = append(out, fmt.Sprintf("line %d: control character in the line; ssh would split it differently from a pinned file; remove it by hand", i+1))
		} else if hasHighByte(f[0]) {
			out = append(out, fmt.Sprintf("line %d: host field has non-ASCII bytes; remove it by hand", i+1))
		} else if f[0][0] == '@' {
			out = append(out, fmt.Sprintf("line %d: %s marker is not supported in a pinned file; remove it by hand", i+1, f[0]))
		} else if strings.ContainsAny(f[0], foreignChars) {
			out = append(out, fmt.Sprintf("line %d: host field uses wildcard, negation, list, [host]:port or hashed syntax, which a pinned file does not support; remove it by hand", i+1))
		} else if len(f) < 3 {
			// hostfile.c steps one byte past the host field's NUL, so a short
			// final line can be parsed with stale bytes of the previous line.
			out = append(out, fmt.Sprintf("line %d: fewer than 3 fields (host, key type, key); ssh may read a short line differently; remove it by hand", i+1))
		} else if !keyTypes[f[1]] {
			// ssh can read "junk ,n1 <key>" as a host list; do not guess.
			out = append(out, fmt.Sprintf("line %d: second field is not a known host key type, so ssh may read the host field differently; remove it by hand", i+1))
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
	if !ownerOK(fi) {
		return fmt.Errorf("pinned host keys directory %s is not owned by you or root", dir)
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
	return checkInfo(fi, path, "file")
}

// checkInfo is the shared file test: regular, owned by us or root, and not
// group/world writable. kind names it in the error ("file", "lock file").
func checkInfo(fi os.FileInfo, path, kind string) error {
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("pinned host keys %s %s is not a regular file (mode %v)", kind, path, fi.Mode())
	}
	if !ownerOK(fi) {
		return fmt.Errorf("pinned host keys %s %s is not owned by you or root", kind, path)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("pinned host keys %s %s has mode %04o: group/world writable; chmod it 0600", kind, path, fi.Mode().Perm())
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
	f, err := os.OpenFile(p.Path+".lock", flags|openNoFollow|openNonBlock, 0o600)
	if err != nil {
		if !write && os.IsNotExist(err) {
			return noUnlock, nil
		}
		return nil, err
	}
	// O_NONBLOCK kept a FIFO from hanging the open; refuse it before flock.
	if fi, serr := f.Stat(); serr != nil {
		_ = f.Close()
		return nil, serr
	} else if cerr := checkInfo(fi, p.Path+".lock", "lock file"); cerr != nil {
		_ = f.Close()
		return nil, cerr
	}
	if err := lockFD(f, write); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { unlockFD(f); _ = f.Close() }, nil
}
