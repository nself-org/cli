// Purpose: copy files to a remote host with scp and rsync, through the exec
//          funnel in exec.go.
// Inputs:  a Target, local and remote paths, caller rsync options.
// Outputs: errors carrying the command output.
// Constraints: argv only; the sdk's options first, then the sdk's "--", then
//              validated operands. No element after "--" starts with '-'.

package remote

import (
	"context"
	"fmt"
	"strings"
)

// CopyTo copies the local file to remote on t.Dest with scp. remote must pass
// ValidateCopyPath (and be non-empty) and local must not look like a
// "host:path" operand or start with '-'. The Target's options are translated
// for scp (see scpOptions): ssh-only flags are dropped and `-p N` becomes
// `-P N`, so no caller option can end scp's option parsing early or make it
// read the sdk's "--" or an operand as an option value.
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
	sopts, err := scpOptions(t.opts())
	if err != nil {
		return err
	}
	args := append(sopts, "--", local, scpOperand(t.Dest, remote))
	cmd, err := t.command(ctx, "scp", args)
	if err != nil {
		return err
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("scp to %s:%s: %w\n%s", t.Dest, remote, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// scpOptions translates ssh-style Target options into the set scp accepts.
// ssh's -T, -a and -x are dropped (scp has no such flags; -T would disable its
// filename checks). ssh's `-p N` (port) becomes scp's `-P N`, because scp's
// own -p means "preserve times" and would leave N as a local operand that ends
// option parsing. -4, -6, -q, -v, -i X, -o V and -oV mean the same in scp. Any
// other element is refused, so scp only ever sees an option scp defines, with
// its value attached or directly after it.
func scpOptions(opts []string) ([]string, error) {
	out := make([]string, 0, len(opts))
	for i := 0; i < len(opts); i++ {
		a := opts[i]
		switch {
		case a == "-T" || a == "-a" || a == "-x":
			// ssh-only: drop.
		case a == "-4" || a == "-6" || a == "-q" || a == "-v":
			out = append(out, a)
		case a == "-o" || a == "-i" || a == "-p":
			if i+1 >= len(opts) {
				return nil, fmt.Errorf("scp option %s has no value", a)
			}
			i++
			if a == "-p" {
				a = "-P"
			}
			out = append(out, a, opts[i])
		case strings.HasPrefix(a, "-o") && len(a) > 2:
			out = append(out, a)
		default:
			return nil, fmt.Errorf("option %q cannot be passed to scp", a)
		}
	}
	return out, nil
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

// Rsync runs `rsync -e "ssh <options>" <args> -- <src> <Dest>:<dst>`. dst is
// the remote path and must pass ValidateCopyPath. The sdk's transport comes
// first, so no caller option can consume it, and the sdk's "--" comes last,
// so no caller option can reach the operands. args is an allowlist (see
// checkRsyncArgs): self-contained flags only, never -e, --rsh, --rsync-path,
// -M, a file-reading flag, a bare "--" or a flag whose value is a separate
// element. Because rsync re-splits -e on spaces, options holding whitespace
// or quotes are refused.
func Rsync(ctx context.Context, t Target, args []string, src, dst string) error {
	if err := t.check(); err != nil {
		return err
	}
	opts := t.opts()
	if err := checkRsyncArgs(args); err != nil {
		return err
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
	argv := append([]string{"-e", "ssh " + strings.Join(opts, " ")}, args...)
	argv = append(argv, "--", src, scpOperand(t.Dest, dst))
	cmd, err := t.command(ctx, "rsync", argv)
	if err != nil {
		return err
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rsync to %s: %w\n%s", t.Dest, err, strings.TrimSpace(string(out)))
	}
	return nil
}
