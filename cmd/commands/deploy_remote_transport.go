package commands

// Purpose: the ssh and rsync calls of the v1.4 `nself deploy <target>` push.
// Inputs:  the workdir, the parsed host, the key path and the host-key policy
//          controlplane.HostKeyOptions returned (shipping, so strict).
// Outputs: combined output and the error of each call.
// Constraints: every argv comes from the deploy transport builder, the one that
//              DeployViaSsh and PushSecrets use; this file builds no "-e" string.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// pushRunner runs the push's ssh and rsync commands against one host.
type pushRunner struct {
	tr      *deploy.Transport
	workdir string
}

// newPushRunner validates the key path and every option token.
func newPushRunner(workdir string, spec remote.HostSpec, keyPath string, policy []string) (*pushRunner, error) {
	abs, err := filepath.Abs(workdir)
	if err != nil {
		return nil, err
	}
	tr, err := deploy.NewTransport(spec, keyPath, policy)
	if err != nil {
		return nil, err
	}
	return &pushRunner{tr: tr, workdir: abs}, nil
}

// rsync copies srcs (relative to the workdir, or absolute) to dest on the host.
// A trailing "/" on a source or on dest is kept.
func (p *pushRunner) rsync(ctx context.Context, flags []string, dest string, srcs ...string) ([]byte, error) {
	abs := make([]string, 0, len(srcs))
	for _, s := range srcs {
		if filepath.IsAbs(s) {
			abs = append(abs, s)
			continue
		}
		joined := filepath.Join(p.workdir, s)
		if s[len(s)-1] == '/' {
			joined += "/"
		}
		abs = append(abs, joined)
	}
	args, err := p.tr.RsyncArgv(flags, dest, abs...)
	if err != nil {
		return nil, err
	}
	rc := exec.CommandContext(ctx, "rsync", args...)
	rc.Dir = p.workdir
	rc.Env = os.Environ()
	return rc.CombinedOutput()
}

// ssh runs command on the host.
func (p *pushRunner) ssh(ctx context.Context, command string) ([]byte, error) {
	sc := exec.CommandContext(ctx, "ssh", p.tr.SSHArgv(command)...)
	sc.Env = os.Environ()
	return sc.CombinedOutput()
}
