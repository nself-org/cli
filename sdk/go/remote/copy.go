// Purpose: copy files to a remote host with scp and rsync, through the exec
//          funnel in exec.go.
// Inputs:  a Target, local and remote paths, caller rsync options.
// Outputs: errors carrying the command output.
// Constraints: argv only; options first, then the sdk's "--", then validated
//              operands. No element after "--" starts with '-'.

package remote

import (
	"context"
	"fmt"
	"strings"
)

// CopyTo copies the local file to remote on t.Dest with scp. remote must pass
// ValidateCopyPath (and be non-empty) and local must not look like a
// "host:path" operand or start with '-'. Options must be `-o` pairs (CIOptions),
// never CISSHFlags: scp has no -T/-a/-x meaning.
func CopyTo(ctx context.Context, t Target, local, remote string) error {
	if err := t.check(); err != nil {
		return err
	}
	if remote == "" {
		return fmt.Errorf("remote path is empty")
	}
	if err := ValidateCopyPath(remote); err != nil {
		return err
	}
	if err := checkLocalOperand(local); err != nil {
		return err
	}
	args := append(t.opts(), "--", local, scpOperand(t.Dest, remote))
	cmd, err := t.command(ctx, "scp", args)
	if err != nil {
		return err
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("scp to %s:%s: %w\n%s", t.Dest, remote, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// checkLocalOperand refuses a local path scp or rsync would read as a remote
// "host:path" (a colon before the first slash), that starts with '-', or that
// holds a newline/NUL.
func checkLocalOperand(local string) error {
	if local == "" || strings.ContainsAny(local, "\n\x00") {
		return fmt.Errorf("local path is empty or holds a newline or NUL")
	}
	if local[0] == '-' {
		return fmt.Errorf("local path %q must not start with '-'; prefix it with ./", local)
	}
	if i := strings.IndexByte(local, ':'); i >= 0 && !strings.Contains(local[:i], "/") {
		return fmt.Errorf("local path %q would be read as a remote operand; prefix it with ./", local)
	}
	return nil
}

// Rsync runs `rsync <args> -e "ssh <options>" -- <src> <Dest>:<dst>`. dst is
// the remote path and must pass ValidateCopyPath. args are the caller's rsync
// options and must each start with '-' in single-element form
// ("--exclude=x", never "--exclude", "x"): a bare "--" or any element that
// does not start with '-' is refused, because rsync stops reading options at
// "--" and would then treat the Target's -e as an operand. Flags that run
// commands or replace the shell (-e, --rsh, --rsync-path, -M, --remote-option)
// are refused. Because rsync re-splits -e on spaces, options holding
// whitespace or quotes are refused.
func Rsync(ctx context.Context, t Target, args []string, src, dst string) error {
	if err := t.check(); err != nil {
		return err
	}
	opts := t.opts()
	for _, a := range args {
		if a == "--" || !strings.HasPrefix(a, "-") {
			return fmt.Errorf("rsync argument %q must be a single-element option starting with '-'", a)
		}
		if rsyncFlagRefused(a) {
			return fmt.Errorf("rsync flag %q is not allowed", a)
		}
	}
	for _, o := range opts {
		if strings.ContainsAny(o, " \t\n\r'\"\\") {
			return fmt.Errorf("ssh option %q cannot be passed through rsync -e", o)
		}
	}
	if dst == "" {
		return fmt.Errorf("remote path is empty")
	}
	if err := ValidateCopyPath(dst); err != nil {
		return err
	}
	if err := checkLocalOperand(src); err != nil {
		return err
	}
	argv := append(append([]string(nil), args...), "-e", "ssh "+strings.Join(opts, " "),
		"--", src, scpOperand(t.Dest, dst))
	cmd, err := t.command(ctx, "rsync", argv)
	if err != nil {
		return err
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rsync to %s: %w\n%s", t.Dest, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// rsyncFlagRefused reports whether a is a flag that replaces the remote shell
// or runs a remote command: -e, -M (alone or inside a short-flag cluster),
// --rsh, --rsync-path, --remote-option, or an abbreviation of one. Rsync callers pass only elements that
// start with '-'.
func rsyncFlagRefused(a string) bool {
	if strings.HasPrefix(a, "--") {
		name := a
		if i := strings.IndexByte(a, '='); i >= 0 {
			name = a[:i]
		}
		// rsync accepts any unambiguous abbreviation of a long option, so a
		// name that is a prefix of a refused option is refused too.
		for _, full := range []string{"--rsh", "--rsync-path", "--remote-option"} {
			if len(name) > 2 && strings.HasPrefix(full, name) || strings.HasPrefix(name, full) {
				return true
			}
		}
		return false
	}
	return len(a) > 1 && a[0] == '-' && strings.ContainsAny(a[1:], "eM")
}
