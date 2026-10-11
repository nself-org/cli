package deploy

// Purpose: the one seam every v1.5 remote step goes through (P7-DEPL-24). The
//          Remote interface runs one remote shell command or one in-place
//          rsync; SSHRemote is the real implementation over sdk/go/remote
//          HostSpec argv, tests supply a recorder.
// Inputs:  a host string, key path and the strict host-key policy.
// Outputs: command stdout, or an error carrying the last stderr text.
// Constraints: ssh and rsync argv come from HostSpec.SSHArgs and SSHOptions
//              only. The host key policy is strict (secrets travel, D13).
//              rsyncArgv refuses any delete flag, so no code path can ask a
//              remote directory to be pruned by rsync.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// Remote runs commands on one remote host.
type Remote interface {
	// Run runs one shell command string and returns its stdout. A non-zero
	// exit returns an error that carries the stderr tail.
	Run(ctx context.Context, command string) ([]byte, error)
	// Rsync copies the paths listed (one relative path per line) in filesFrom,
	// taken relative to srcDir, into destDir on the host, in place. flags are
	// extra rsync flags; a delete flag is refused.
	Rsync(ctx context.Context, flags []string, filesFrom, srcDir, destDir string) error
	// Label names the host for state records and messages.
	Label() string
}

// SSHRemote is the real Remote.
type SSHRemote struct {
	Spec remote.HostSpec
	// Opts are the ssh options placed before the HostSpec argv (-i, host key
	// policy, ForwardAgent=no).
	Opts []string
}

// NewSSHRemote parses host (E484 on a bad string) and builds the strict host
// key policy for it (E487 for an unknown key).
func NewSSHRemote(ctx context.Context, host, keyPath, env, server string) (*SSHRemote, error) {
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		return nil, errs.New("E484", err.Error())
	}
	if keyPath != "" && !SSHKeyRe.MatchString(keyPath) {
		return nil, fmt.Errorf("ssh key path contains unsafe characters (got %q): only [a-zA-Z0-9/_.~-] allowed", keyPath)
	}
	policy, err := HostKeyOptions(ctx, spec.String(), env, server, true)
	if err != nil {
		return nil, err
	}
	opts := sshOpts(keyPath, policy)
	return &SSHRemote{Spec: spec, Opts: opts}, nil
}

// sshOpts are the ssh options placed before the HostSpec argv. BatchMode: the
// children run in their own process group, so a prompt on the tty would stop
// them (SIGTTIN) and hang the deploy; auth must be non-interactive.
func sshOpts(keyPath string, policy []string) []string {
	var opts []string
	if keyPath != "" {
		opts = append(opts, "-i", keyPath)
	}
	opts = append(opts, policy...)
	return append(opts, "-o", "ForwardAgent=no", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10")
}

// Label returns the canonical host string.
func (r *SSHRemote) Label() string { return r.Spec.String() }

// Run implements Remote.
func (r *SSHRemote) Run(ctx context.Context, command string) ([]byte, error) {
	args := append(append([]string{}, r.Opts...), r.Spec.SSHArgs()...)
	args = append(args, command)
	cmd, err := remote.Command(ctx, "ssh", args...)
	if err != nil {
		return nil, err
	}
	cmd.Env = os.Environ()
	ownProcessGroup(cmd)
	var se bytes.Buffer
	cmd.Stderr = &se
	out, err := cmd.Output()
	if err != nil {
		msg := tail(se.String())
		if strings.Contains(se.String(), "Permission denied") {
			msg += " (deploys run non-interactively: use ssh-agent or an unencrypted deploy key)"
		}
		return out, fmt.Errorf("ssh %s: %w: %s", r.Spec.Dest(), err, msg)
	}
	return out, nil
}

// Rsync implements Remote.
func (r *SSHRemote) Rsync(ctx context.Context, flags []string, filesFrom, srcDir, destDir string) error {
	args, err := rsyncArgv(r.Spec, r.Opts, flags, filesFrom, srcDir, destDir)
	if err != nil {
		return err
	}
	cmd, err := remote.Command(ctx, "rsync", args...)
	if err != nil {
		return err
	}
	cmd.Env = os.Environ()
	ownProcessGroup(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rsync to %s: %w\n%s", r.Spec.Dest(), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// rsyncFlagOK is the allowlist for caller flags: no delete flag, no -e, no
// source removal can pass.
var rsyncFlagOK = regexp.MustCompile(`^(-[zcvn]|--itemize-changes|--chmod=[A-Za-z0-9,+=-]+|--exclude=[^\s]+)$`)

// rsyncArgv builds the rsync argv for an in-place copy of the files listed in
// filesFrom. There is never a delete flag: caller flags pass an allowlist.
func rsyncArgv(spec remote.HostSpec, sshOpts, flags []string, filesFrom, srcDir, destDir string) ([]string, error) {
	if err := remote.ValidateCopyPath(destDir); err != nil {
		return nil, err
	}
	all := append(append([]string{}, sshOpts...), spec.SSHOptions()...)
	for _, opt := range all {
		if strings.ContainsAny(opt, " \t\n\r'\"\\") {
			return nil, fmt.Errorf("ssh option %q is unsafe in rsync -e", opt)
		}
	}
	for _, f := range flags {
		if !rsyncFlagOK.MatchString(f) {
			return nil, fmt.Errorf("rsync flag %q is not allowed (no delete, no transport override)", f)
		}
	}
	target := spec.String()
	if spec.Port != 0 {
		target = strings.TrimSuffix(target, ":"+strconv.Itoa(spec.Port))
	}
	// --keep-dirlinks: a symlinked nginx/ on the host is followed, never replaced by a real dir.
	args := []string{"-a", "--keep-dirlinks", "--no-implied-dirs", "-e", "ssh " + strings.Join(all, " ")}
	args = append(args, flags...)
	args = append(args, "--files-from="+filesFrom, "--", strings.TrimSuffix(srcDir, "/")+"/", target+":"+strings.TrimSuffix(destDir, "/")+"/")
	return args, nil
}

// tail returns the last non-empty line of s.
func tail(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// shScript wraps a POSIX script so the remote login shell, whatever it is,
// runs it with sh.
func shScript(script string) string { return "sh -c " + remote.ShellQuote(script) }

// q is remote.ShellQuote.
func q(s string) string { return remote.ShellQuote(s) }

// composeBase renders `docker compose -f F... --env-file E...` over the
// resolved remote manifests, every value quoted, in the order
// docker.Compose.buildBaseArgs uses.
func composeBase(files, envFiles []string) string {
	parts := []string{"docker compose"}
	for _, f := range files {
		parts = append(parts, "-f", q(f))
	}
	for _, e := range envFiles {
		parts = append(parts, "--env-file", q(e))
	}
	return strings.Join(parts, " ")
}
