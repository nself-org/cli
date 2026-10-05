// Purpose: run commands on, and copy files to, a remote host with the
//          operator's OpenSSH, through the exec funnel in exec.go.
// Inputs:  a Target (destination, key, option set, environment) and the
//          command, files or paths to act on.
// Outputs: command output and errors; a Session for streaming.
// Constraints: argv only. Every operand follows the sdk's "--" and is
//              validated before exec; the caller's own options and operands
//              can never precede or replace the Target's options. Remote
//              commands assume a POSIX remote shell (see RunArgv).

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

// sshArgs builds options, "--", destination, command. A command starting with
// '-' is refused so no element after "--" looks like an option.
func (t Target) sshArgs(command string) ([]string, error) {
	if err := t.check(); err != nil {
		return nil, err
	}
	if strings.HasPrefix(command, "-") {
		return nil, fmt.Errorf("remote command must not start with '-' (got %q)", command)
	}
	args := append(t.opts(), "--", t.Dest)
	if command != "" {
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
//
// ShellQuote's single quotes are POSIX-shell quoting: the remote login shell
// must be a POSIX shell (sh, bash, dash, zsh). Under fish a backslash inside
// single quotes still escapes, so RunArgv refuses any element holding a
// backslash instead of guessing the remote shell; pass such data in a file.
func RunArgv(ctx context.Context, t Target, argv ...string) (string, error) {
	if len(argv) == 0 {
		return "", fmt.Errorf("remote: empty command")
	}
	quoted := make([]string, len(argv))
	for i, a := range argv {
		if strings.ContainsRune(a, '\\') {
			return "", fmt.Errorf("remote: argument %d holds a backslash, which is not quoted safely for every remote shell", i)
		}
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
