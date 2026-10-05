// Purpose: run commands on, and copy files to, a remote host with the
//          operator's OpenSSH, through the exec funnel in exec.go.
// Inputs:  a Target (destination, key, option set, environment) and the
//          command, files or paths to act on.
// Outputs: command output and errors; a Session for streaming.
// Constraints: argv only. With Compat false, operands follow "--" and are
//              validated before exec. With Compat true the argv is exactly
//              what cli deploy has always produced (no "--", inherited env).

package remote

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Target is a remote destination plus how to reach it.
type Target struct {
	// Dest is "host" or "user@host" (no path component).
	Dest string
	// KeyPath is the identity file, used only when Options is nil.
	KeyPath string
	// Options are the argv elements placed before the destination. Nil means
	// BaseOptions(KeyPath); use a non-nil empty slice for "none". CI callers
	// pass CISSHFlags() plus CIOptions(...) (ssh) or CIOptions(...) (scp,
	// rsync -e).
	Options []string
	// Env is the process environment. Nil means EnvAllowlist().
	Env []string
	// Compat reproduces cli deploy's historical argv and relaxes the
	// destination check to "no leading '-', no whitespace". Internal use.
	Compat bool
}

// DefaultKeyPath returns the SSH key path from NSELF_DEPLOY_KEY_PATH or
// NSELF_DEPLOY_SSH_KEY (legacy), falling back to ~/.ssh/id_ed25519.
func DefaultKeyPath() string {
	if k := os.Getenv("NSELF_DEPLOY_KEY_PATH"); k != "" {
		return k
	}
	if k := os.Getenv("NSELF_DEPLOY_SSH_KEY"); k != "" {
		return k
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "id_ed25519")
}

// BaseOptions returns the common SSH flags cli deploy uses for every remote
// operation (the historical sshBaseArgs set, byte for byte).
func BaseOptions(keyPath string) []string {
	return []string{
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ForwardAgent=no",
	}
}

// ShellQuote wraps s in single quotes for safe inclusion in a remote shell
// command string, escaping any embedded single quotes POSIX-style.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (t Target) opts() []string {
	if t.Options != nil {
		return append([]string(nil), t.Options...)
	}
	return BaseOptions(t.KeyPath)
}

func (t Target) check() error {
	if t.Compat {
		return ValidateLegacyDest(t.Dest)
	}
	if err := ValidateDest(t.Dest); err != nil {
		return err
	}
	return checkOptions(t.Options)
}

func (t Target) command(ctx context.Context, tool string, args []string) (*exec.Cmd, error) {
	cmd, err := Command(ctx, tool, args...)
	if err != nil {
		return nil, err
	}
	if t.Env != nil {
		cmd.Env = t.Env
	}
	return cmd, nil
}

// sshArgs builds options, "--" (unless Compat), destination, command.
func (t Target) sshArgs(command string) ([]string, error) {
	if err := t.check(); err != nil {
		return nil, err
	}
	args := t.opts()
	if !t.Compat {
		args = append(args, "--")
	}
	args = append(args, t.Dest)
	if command != "" || t.Compat { // Compat always sends the element, as cli deploy did
		args = append(args, command)
	}
	return args, nil
}

// Run runs command on the remote host via ssh and returns combined
// stdout+stderr, trimmed. command is passed as ONE argv element; ssh hands it
// to the remote shell, so a caller that builds it must quote with ShellQuote
// (or use RunArgv). Errors name the destination and carry the output.
func Run(ctx context.Context, t Target, command string) (string, error) {
	args, err := t.sshArgs(command)
	if err != nil {
		return "", err
	}
	cmd, err := t.command(ctx, "ssh", args)
	if err != nil {
		return "", err
	}
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		return trimmed, fmt.Errorf("remote command on %s failed: %w\n%s", t.Dest, err, trimmed)
	}
	return trimmed, nil
}

// RunArgv runs argv on the remote host, quoting every element with
// ShellQuote so none can be parsed as shell syntax by the remote shell.
func RunArgv(ctx context.Context, t Target, argv ...string) (string, error) {
	if len(argv) == 0 {
		return "", fmt.Errorf("remote: empty command")
	}
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = ShellQuote(a)
	}
	return Run(ctx, t, strings.Join(quoted, " "))
}

// Session is a started ssh process with its pipes.
type Session struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
	cmd    *exec.Cmd
}

// Start starts command on the remote host and returns the running Session.
// An empty command starts a login shell session (callers on the CI path pass
// the agent command).
func Start(ctx context.Context, t Target, command string) (*Session, error) {
	args, err := t.sshArgs(command)
	if err != nil {
		return nil, err
	}
	cmd, err := t.command(ctx, "ssh", args)
	if err != nil {
		return nil, err
	}
	s := &Session{cmd: cmd}
	if s.Stdin, err = cmd.StdinPipe(); err != nil {
		return nil, err
	}
	if s.Stdout, err = cmd.StdoutPipe(); err != nil {
		return nil, err
	}
	if s.Stderr, err = cmd.StderrPipe(); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ssh to %s: %w", t.Dest, err)
	}
	return s, nil
}

// Wait waits for the process to exit.
func (s *Session) Wait() error { return s.cmd.Wait() }

// Kill kills the process.
func (s *Session) Kill() error {
	if s.cmd.Process == nil {
		return nil
	}
	return s.cmd.Process.Kill()
}

// CopyTo copies the local file to remote on t.Dest with scp. Unless t.Compat,
// remote must pass ValidateRemotePath (and be non-empty) and local must not
// look like a "host:path" operand. Options must be `-o` pairs (CIOptions),
// never CISSHFlags: scp has no -T/-a/-x meaning.
func CopyTo(ctx context.Context, t Target, local, remote string) error {
	if err := t.check(); err != nil {
		return err
	}
	args := t.opts()
	if !t.Compat {
		if remote == "" {
			return fmt.Errorf("remote path is empty")
		}
		if err := ValidateRemotePath(remote); err != nil {
			return err
		}
		if err := checkLocalOperand(local); err != nil {
			return err
		}
		args = append(args, "--")
	}
	args = append(args, local, t.Dest+":"+remote)
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
// "host:path" (a colon before the first slash), or that holds a newline/NUL.
func checkLocalOperand(local string) error {
	if local == "" || strings.ContainsAny(local, "\n\x00") {
		return fmt.Errorf("local path is empty or holds a newline or NUL")
	}
	if i := strings.IndexByte(local, ':'); i >= 0 && !strings.Contains(local[:i], "/") {
		return fmt.Errorf("local path %q would be read as a remote operand; prefix it with ./", local)
	}
	return nil
}

// Rsync runs `rsync <args> -e "ssh <options>" -- <src> <Dest>:<dst>`. dst is
// the remote path and must pass ValidateRemotePath unless t.Compat. args are
// the caller's rsync flags; flags that run commands or replace the shell
// (-e, --rsh, --rsync-path, -M, --remote-option) are refused. Because rsync
// re-splits -e on spaces, options holding whitespace or quotes are refused.
func Rsync(ctx context.Context, t Target, args []string, src, dst string) error {
	if err := t.check(); err != nil {
		return err
	}
	opts := t.opts()
	argv := append([]string(nil), args...)
	if !t.Compat {
		for _, a := range args {
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
		if err := ValidateRemotePath(dst); err != nil {
			return err
		}
		if err := checkLocalOperand(src); err != nil {
			return err
		}
	}
	argv = append(argv, "-e", "ssh "+strings.Join(opts, " "))
	if !t.Compat {
		argv = append(argv, "--")
	}
	argv = append(argv, src, t.Dest+":"+dst)
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
// --rsh, --rsync-path, --remote-option. Operands (no leading '-') pass here
// and are validated separately.
func rsyncFlagRefused(a string) bool {
	if strings.HasPrefix(a, "--") {
		return strings.HasPrefix(a, "--rsh") || strings.HasPrefix(a, "--rsync-path") ||
			strings.HasPrefix(a, "--remote-option")
	}
	return len(a) > 1 && a[0] == '-' && strings.ContainsAny(a[1:], "eM")
}
