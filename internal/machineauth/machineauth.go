// Package machineauth is the machine-surface bearer token: where it comes
// from, how it is created, how a request is checked against it, and which
// bind addresses may carry it.
//
// Purpose: P7-SURF-25 (EPIC SURF D4). The MCP server (and later the HTTP
//
//	control API) needs one token source with one set of safety rules, so no
//	command re-implements them.
//
// Inputs: the NSELF_MCP_TOKEN environment variable, the file
//
//	~/.nself/mcp-token, and the user's home directory.
//
// Outputs: a token plus its source ("env", "file" or "generated"), an
//
//	http.Handler middleware, and bind-address checks. Every refusal is an
//	errs E426 error.
//
// Constraints: Lookup never writes anything; only Resolve creates the token
//
//	file, and only when Lookup finds none. A token value is never written to
//	any stream (Resolve prints the file path only). Layer L1: this package
//	imports only the standard library and internal/errs.
//
// SPORT: CLI-CAP-MACHINE-AUTH-001
package machineauth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// EnvVar is the environment variable that carries the machine token.
const EnvVar = "NSELF_MCP_TOKEN"

// Token sources reported by Lookup and Resolve.
const (
	SourceEnv       = "env"
	SourceFile      = "file"
	SourceGenerated = "generated"
)

// tokenBytes is the size of a generated token before hex encoding.
const tokenBytes = 32

// unixPerms reports whether POSIX permission bits are meaningful here.
func unixPerms() bool { return runtime.GOOS != "windows" }

// TokenPath returns the token file path, ~/.nself/mcp-token.
func TokenPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errs.Wrap("E426", "cannot locate the home directory for the machine token file", err)
	}
	return filepath.Join(home, ".nself", "mcp-token"), nil
}

// Lookup finds the configured token without creating anything. The
// environment variable wins (source "env"); otherwise an existing token file
// is used (source "file"). With neither it returns "", "", nil. A token file
// that is a symlink, not a regular file, readable by group or others
// (non-Windows) or empty is refused with E426.
func Lookup() (tok, source string, err error) {
	if v := os.Getenv(EnvVar); v != "" {
		return v, SourceEnv, nil
	}
	path, perr := TokenPath()
	if perr != nil {
		// No home directory means no token file; Resolve reports it when it
		// has to create one.
		return "", "", nil
	}
	lst, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", nil
		}
		return "", "", errs.Wrap("E426", fmt.Sprintf("cannot stat the machine token file %s", path), err)
	}
	if !lst.Mode().IsRegular() {
		return "", "", errs.New("E426", fmt.Sprintf("machine token file %s is not a regular file (symlinks are refused)", path)).
			WithFix("Remove it and let nself create it, or replace it with a regular file at mode 0600.")
	}
	if unixPerms() && lst.Mode().Perm()&0o077 != 0 {
		return "", "", errs.New("E426", fmt.Sprintf("machine token file %s is readable by other users (mode %04o)", path, lst.Mode().Perm())).
			WithFix("Run: chmod 600 " + path)
	}
	if err := checkTokenLocation(path, lst); err != nil {
		return "", "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", "", errs.Wrap("E426", fmt.Sprintf("cannot read the machine token file %s", path), err)
	}
	defer func() { _ = f.Close() }()
	if fst, serr := f.Stat(); serr != nil || !os.SameFile(lst, fst) {
		return "", "", errs.New("E426", fmt.Sprintf("machine token file %s changed while it was being read", path))
	}
	raw, err := io.ReadAll(io.LimitReader(f, 1<<16))
	if err != nil {
		return "", "", errs.Wrap("E426", fmt.Sprintf("cannot read the machine token file %s", path), err)
	}
	tok = strings.TrimSpace(string(raw))
	if tok == "" {
		return "", "", errs.New("E426", fmt.Sprintf("machine token file %s is empty", path)).
			WithFix("Delete it to have nself generate a token, or put a token in it at mode 0600.")
	}
	return tok, SourceFile, nil
}

// checkTokenLocation refuses a token file that another user could replace or
// that another user owns (non-Windows): a parent directory writable by group
// or others without the sticky bit, or an owner other than the effective user.
// It reads metadata only.
func checkTokenLocation(path string, lst os.FileInfo) error {
	if !unixPerms() {
		return nil
	}
	dir := filepath.Dir(path)
	if dst, err := os.Stat(dir); err != nil {
		return errs.Wrap("E426", fmt.Sprintf("cannot stat %s", dir), err)
	} else if dst.Mode().Perm()&0o022 != 0 && dst.Mode()&os.ModeSticky == 0 {
		return errs.New("E426", fmt.Sprintf("%s is writable by other users (mode %04o), so the machine token file in it could be replaced", dir, dst.Mode().Perm())).
			WithFix("Run: chmod go-w " + dir)
	}
	if uid, ok := fileOwner(lst); ok && uid != currentUID() {
		return errs.New("E426", fmt.Sprintf("machine token file %s is owned by uid %d, not the current user", path, uid)).
			WithFix(fmt.Sprintf("Run: chown %d %s", currentUID(), path))
	}
	return nil
}

// Resolve is Lookup, then token generation when nothing is configured. It
// reports the new file path (never the token) on standard error.
func Resolve() (tok, source string, generated bool, err error) {
	return ResolveTo(os.Stderr)
}

// ResolveTo is Resolve with the notice writer injected.
func ResolveTo(notice io.Writer) (tok, source string, generated bool, err error) {
	tok, source, err = Lookup()
	if err != nil || tok != "" {
		return tok, source, false, err
	}
	path, err := TokenPath()
	if err != nil {
		return "", "", false, err
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return "", "", false, err
	}
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", false, errs.Wrap("E426", "cannot generate a machine token", err)
	}
	tok = hex.EncodeToString(buf)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return lookupAfterRace()
		}
		return "", "", false, errs.Wrap("E426", fmt.Sprintf("cannot create the machine token file %s", path), err)
	}
	_, werr := f.WriteString(tok + "\n")
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(path)
		return "", "", false, errs.Wrap("E426", fmt.Sprintf("cannot write the machine token file %s", path), errors.Join(werr, cerr))
	}
	if notice != nil {
		_, _ = fmt.Fprintf(notice, "nself: generated a machine token for the MCP server and saved it to %s\n  read it with: cat %s\n", path, path)
	}
	return tok, SourceGenerated, true, nil
}

// lookupAfterRace re-reads the file a concurrent start created. The other
// process may not have written it yet, so an empty file is retried briefly.
func lookupAfterRace() (string, string, bool, error) {
	var tok, src string
	var err error
	for i := 0; i < 10; i++ {
		tok, src, err = Lookup()
		if err == nil && tok != "" {
			return tok, src, false, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err == nil {
		err = errs.New("E426", "machine token file appeared but holds no token")
	}
	return "", "", false, err
}

// ensureDir creates dir with mode 0700 when missing. On non-Windows an
// existing directory is narrowed to 0700 (group and other bits removed, never
// widened: other nself commands create ~/.nself at 0755 first); when that
// fails and group or others can still write to it, it is refused.
func ensureDir(dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return errs.Wrap("E426", fmt.Sprintf("cannot create %s", dir), err)
	}
	st, err := os.Stat(dir)
	if err != nil {
		return errs.Wrap("E426", fmt.Sprintf("cannot stat %s", dir), err)
	}
	if !st.IsDir() {
		return errs.New("E426", fmt.Sprintf("%s exists and is not a directory", dir))
	}
	if unixPerms() && st.Mode().Perm()&0o077 != 0 {
		if os.Chmod(dir, st.Mode().Perm()&^0o077) == nil {
			return nil
		}
	}
	if unixPerms() && st.Mode().Perm()&0o022 != 0 {
		return errs.New("E426", fmt.Sprintf("%s is writable by other users (mode %04o), so a token stored there could be replaced", dir, st.Mode().Perm())).
			WithFix("Run: chmod go-w " + dir)
	}
	return nil
}
