package deploy

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// windowsDrivePath identifies an absolute drive path on any host, including
// when tests exercise Windows argv on a POSIX host. Drive-relative paths are
// deliberately excluded.
func windowsDrivePath(p string) bool {
	return len(p) >= 3 && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) && p[1] == ':' && (p[2] == '/' || p[2] == '\\')
}

// sshKeyPath normalizes an already validated key path for ssh and rsync -e.
// Only absolute Windows drive paths admit backslash separators.
func sshKeyPath(p string) string {
	if windowsDrivePath(p) {
		p = strings.ReplaceAll(p, `\`, "/")
	}
	return filepath.ToSlash(p)
}

// rsyncLocalOperand maps native local paths to the installed POSIX runtime.
// Inputs: a local source or file list. Outputs: an absolute local rsync path.
// Constraints: no shell; cygpath must match rsync's MSYS/Cygwin runtime. Its
// mount table is authoritative, so no drive mount prefix is hard-coded.
func rsyncLocalOperand(p string) (string, error) {
	local, err := localOperand(p)
	if err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" && !windowsDrivePath(local) {
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
