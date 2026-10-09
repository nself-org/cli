package access

// Purpose: the production Transport — runs `ssh` against a real host to
// read, back up, and write its authorized_keys file. This is the only
// implementation `nself access` wires up outside tests; the test suite
// exercises grant/revoke/list exclusively through LocalFileTransport
// (transport_local.go) so it never opens a network connection to any host,
// staging or production included.
// Inputs: an SSH connection target and the operator's local identity file.
// Outputs: remote command execution over ssh.
// Constraints: never reads the identity file's content — only passes its
// path to the ssh binary via -i — and never logs command output beyond what
// the caller explicitly does with a Read result (public key material only).

import (
	"bytes"
	"context"
	"fmt"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SSHTransport implements Transport by running the ssh binary against a
// real host.
type SSHTransport struct {
	// Host is "[user@]host" — the ssh connection target, e.g. "root@5.75.235.42".
	Host string

	// IdentityPath is the local private key used to authenticate the SSH
	// connection (the operator's own key, distinct from any key being
	// granted or revoked).
	IdentityPath string

	// RemotePath is the authorized_keys path on the remote host. Defaults to
	// "~/.ssh/authorized_keys" (relative to whichever account Host connects
	// as) when empty.
	RemotePath string
}

func (t *SSHTransport) remotePath() string {
	if t.RemotePath != "" {
		return t.RemotePath
	}
	return "~/.ssh/authorized_keys"
}

func (t *SSHTransport) Describe() string { return t.Host }

func (t *SSHTransport) sshArgs() ([]string, error) {
	spec, err := remote.ParseHostSpec(t.Host)
	if err != nil {
		return nil, err
	}
	args := []string{
		"-i", t.IdentityPath,
		"-o", "ForwardAgent=no",
		"-o", "BatchMode=yes",
	}
	// compat.V15(P7-DEPL-13): accept-new -> strict host key checking for access writes.
	if compat.V15() {
		home, _ := os.UserHomeDir()
		args = append(args, "-o", "StrictHostKeyChecking=yes", "-o", "GlobalKnownHostsFile="+filepath.Join(home, ".config", "nself", "deploy_known_hosts"))
		if spec.Port != 0 {
			alias := "nself-" + strings.ReplaceAll(spec.Host, ":", "-") + "-" + strconv.Itoa(spec.Port)
			args = append(args, "-o", "HostKeyAlias="+alias)
		}
	} else {
		args = append(args, "-o", "StrictHostKeyChecking=accept-new")
	}
	return append(args, spec.SSHArgs()...), nil
}

// runRemote executes command on the remote host and returns its stdout.
func (t *SSHTransport) runRemote(ctx context.Context, command string) ([]byte, error) {
	args, err := t.sshArgs()
	if err != nil {
		return nil, err
	}
	args = append(args, command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ssh %s %q: %w: %s", t.Host, command, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Read returns the remote authorized_keys content, or (nil, nil) if the file
// does not exist on the remote host.
func (t *SSHTransport) Read(ctx context.Context) ([]byte, error) {
	remote := t.remotePath()
	out, err := t.runRemote(ctx, fmt.Sprintf("cat %s 2>/dev/null || true", shellQuote(remote)))
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Backup copies the remote file to a timestamped sibling and returns that
// remote path, or "" if there was nothing to back up.
func (t *SSHTransport) Backup(ctx context.Context) (string, error) {
	remote := t.remotePath()
	backup := remote + ".bak." + time.Now().UTC().Format("20060102T150405Z")
	script := fmt.Sprintf(
		"if [ -f %s ]; then cp -p %s %s && echo %s; fi",
		shellQuote(remote), shellQuote(remote), shellQuote(backup), shellQuote(backup))
	out, err := t.runRemote(ctx, script)
	if err != nil {
		return "", fmt.Errorf("backup %s on %s: %w", remote, t.Host, err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return "", nil
	}
	return backup, nil
}

// Write replaces the remote authorized_keys content, creating its parent
// directory (0700) first, and leaves the file at 0600.
func (t *SSHTransport) Write(ctx context.Context, content []byte) error {
	remote := t.remotePath()
	dir := path.Dir(remote)
	script := fmt.Sprintf(
		"mkdir -p %s && chmod 700 %s && cat > %s && chmod 600 %s",
		shellQuote(dir), shellQuote(dir), shellQuote(remote), shellQuote(remote))

	args, err := t.sshArgs()
	if err != nil {
		return err
	}
	args = append(args, script)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdin = bytes.NewReader(content)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("write %s on %s: %w: %s", remote, t.Host, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// shellQuote wraps s in single quotes for safe interpolation into a remote
// shell command, escaping any embedded single quote.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
