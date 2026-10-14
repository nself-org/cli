package deploy

// Purpose: the exported face of the ssh option builder, for deploy callers
//          outside this package (the v1.4 `nself deploy <target>` push).
// Inputs:  a parsed remote.HostSpec, an optional key path and the host-key
//          policy HostKeyPolicy returned.
// Outputs: ssh and rsync argv with every option token checked.
// Constraints: the same builder DeployViaSsh and PushSecrets use, so no caller
//              keeps its own "-e" string.

import "github.com/nself-org/cli/sdk/go/v2/remote"

// Transport builds ssh and rsync argv for one host.
type Transport struct{ t sshTransport }

// NewTransport validates keyPath and every option token and returns the builder.
func NewTransport(spec remote.HostSpec, keyPath string, policy []string) (*Transport, error) {
	t, err := newSSHTransport(spec, keyPath, policy)
	if err != nil {
		return nil, err
	}
	return &Transport{t: t}, nil
}

// SSHArgv is the ssh argv for command: options, port, "--", destination, command.
func (x *Transport) SSHArgv(command string) []string { return x.t.sshArgv(command) }

// RsyncArgv is the rsync argv that copies srcs (absolute local paths) to dest
// on the host. flags are long options such as "--delete". dest must pass
// remote.ValidateCopyPath.
func (x *Transport) RsyncArgv(flags []string, dest string, srcs ...string) ([]string, error) {
	return x.t.rsyncMultiArgv(flags, dest, srcs...)
}

// Operand is the "[user@]host:path" rsync and scp operand (no port).
func (x *Transport) Operand(path string) string { return x.t.operand(path) }
