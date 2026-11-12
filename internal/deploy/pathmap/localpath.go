package pathmap

// Purpose: the local-path operands deploy hands to ssh, scp and rsync: Windows
//          drive detection, key-path normalisation, absolute local operands and
//          their rsync (MSYS/Cygwin) form. Moved out of internal/deploy as is.
// Inputs:  a local path.
// Outputs: the path in the form the tool reads, or an error.
// Constraints: no shell; cygpath must match rsync's MSYS/Cygwin runtime.

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// WindowsDrivePath identifies an absolute drive path on any host, including
// when tests exercise Windows argv on a POSIX host. Drive-relative paths are
// deliberately excluded.
func WindowsDrivePath(p string) bool {
	return len(p) >= 3 && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) && p[1] == ':' && (p[2] == '/' || p[2] == '\\')
}

// SSHKeyPath normalizes an already validated key path for ssh and rsync -e.
// Only absolute Windows drive paths admit backslash separators.
func SSHKeyPath(p string) string {
	if WindowsDrivePath(p) {
		p = strings.ReplaceAll(p, `\`, "/")
	}
	return filepath.ToSlash(p)
}

// LocalOperand makes a relative p absolute, so scp and rsync never read it as
// a "host:path" operand or an option. Absolute paths retain their native
// form for scp; RsyncLocalOperand also maps Windows paths for POSIX rsync.
// Empty paths, newlines and NUL are refused.
func LocalOperand(p string) (string, error) {
	if p == "" || strings.ContainsAny(p, "\n\x00") {
		return "", fmt.Errorf("local path %q is empty or contains a newline or NUL", p)
	}
	if strings.HasPrefix(p, "/") || filepath.IsAbs(p) || WindowsDrivePath(p) {
		return p, nil
	}
	return filepath.Abs(p)
}

// RsyncLocalOperand maps native local paths to the installed POSIX runtime.
// Inputs: a local source or file list. Outputs: an absolute local rsync path.
// Its mount table is authoritative, so no drive mount prefix is hard-coded.
func RsyncLocalOperand(p string) (string, error) {
	local, err := LocalOperand(p)
	if err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" && !WindowsDrivePath(local) {
		return local, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "cygpath", "-u", "--", local).Output()
	if err != nil {
		return "", fmt.Errorf("convert Windows local path %q for rsync: install cygpath from the same MSYS/Cygwin runtime as rsync: %w", local, err)
	}
	converted := strings.TrimRight(string(out), "\r\n")
	if !strings.HasPrefix(converted, "/") || strings.ContainsAny(converted, "\r\n\x00") {
		return "", fmt.Errorf("cygpath returned an invalid absolute local path for %q", local)
	}
	if strings.HasSuffix(local, "/") || strings.HasSuffix(local, `\`) {
		converted = strings.TrimSuffix(converted, "/") + "/"
	}
	return converted, nil
}
