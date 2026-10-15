package deploy

// Purpose: one ssh option builder shared by DeployViaSsh (ssh, rsync -e) and
//          PushSecrets (scp), so the three tools can never drift apart.
// Inputs:  a parsed remote.HostSpec, an optional key path and a host-key
//          policy (the "-o ..." tokens hostKeyPolicy returned).
// Outputs: argv slices for ssh, rsync and scp; every element is checked.
// Constraints: argv only, no shell. rsync -e re-splits its value on blanks.
//              Only canonical quoted pin-file tokens allow blanks and quotes;
//              all other tokens exclude blanks, quotes, backslash and backtick.
//              Local operands are made absolute so
//              scp and rsync never read them as host:path or as an option.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// SSHKeyRe is the safe alphabet for an SSH key path. The one definition: the
// legacy push in cmd/commands aliases it.
var SSHKeyRe = regexp.MustCompile(`^[a-zA-Z0-9/_.~-]+$`)

// sshTransport is the ssh option set for one host. opts holds [-i key], the
// host-key policy and ForwardAgent=no; the port is NOT in opts (it is added
// per tool: "-p N" for ssh and rsync -e, "-P N" for scp).
type sshTransport struct {
	spec remote.HostSpec
	opts []string
}

// checkDeployKeyPath accepts "" (no -i) or a path in the SSHKeyRe alphabet
// that does not start with '-'.
func checkDeployKeyPath(p string) error {
	if p == "" {
		return nil
	}
	if !SSHKeyRe.MatchString(p) {
		return fmt.Errorf("ssh key path contains unsafe characters (got %q): only [a-zA-Z0-9/_.~-] allowed", p)
	}
	if p[0] == '-' {
		return fmt.Errorf("ssh key path must not start with '-' (got %q)", p)
	}
	return nil
}

// checkTransportToken permits canonical SDK pin-file options. All other tokens
// refuse blanks, controls, quotes, backslashes and backticks, which rsync splits.
func checkTransportToken(tok string) error {
	if strings.HasPrefix(tok, `GlobalKnownHostsFile="`) && strings.HasSuffix(tok, `"`) {
		path := strings.TrimSuffix(strings.TrimPrefix(tok, `GlobalKnownHostsFile="`), `"`)
		canonical, err := remote.GlobalKnownHostsFileOption(path)
		if err == nil && canonical == tok {
			return nil
		}
		return fmt.Errorf("invalid quoted pin file option %q", tok)
	}
	if tok == "" {
		return fmt.Errorf("ssh option token is empty")
	}
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if c <= ' ' || c == 0x7f || c == '\'' || c == '"' || c == '\\' || c == '`' {
			return fmt.Errorf("ssh option token %q contains an unsafe character at byte %d", tok, i)
		}
	}
	return nil
}

// newSSHTransport builds the option list: -i keyPath (only when keyPath is
// set), policy, then ForwardAgent=no. Every token, and the spec's port
// options, are validated.
func newSSHTransport(spec remote.HostSpec, keyPath string, policy []string) (sshTransport, error) {
	if err := checkDeployKeyPath(keyPath); err != nil {
		return sshTransport{}, err
	}
	var opts []string
	if keyPath != "" {
		opts = append(opts, "-i", keyPath)
	}
	opts = append(opts, policy...)
	opts = append(opts, "-o", "ForwardAgent=no")
	for _, tok := range append(append([]string{}, opts...), spec.SSHOptions()...) {
		if err := checkTransportToken(tok); err != nil {
			return sshTransport{}, err
		}
	}
	return sshTransport{spec: spec, opts: opts}, nil
}

// sshArgv is the ssh argv: options, port, "--", destination, command.
func (t sshTransport) sshArgv(command string) []string {
	args := append(append([]string{}, t.opts...), t.spec.SSHArgs()...)
	return append(args, command)
}

// rsyncArgv is the rsync argv for copying src to dest on the host. dest must
// pass remote.ValidateCopyPath and be non-empty; src becomes absolute.
func (t sshTransport) rsyncArgv(src, dest string) ([]string, error) {
	return t.rsyncMultiArgv(nil, dest, src)
}

// rsyncMultiArgv is rsyncArgv for several sources and extra rsync flags (each
// must start with "--"). A source that is already absolute is kept as it is, so
// a trailing "/" (copy the contents of a directory) survives.
func (t sshTransport) rsyncMultiArgv(flags []string, dest string, srcs ...string) ([]string, error) {
	if dest == "" {
		return nil, fmt.Errorf("remote path is empty")
	}
	if err := remote.ValidateCopyPath(dest); err != nil {
		return nil, err
	}
	if len(srcs) == 0 {
		return nil, fmt.Errorf("rsync needs at least one source")
	}
	args := []string{"-az"}
	for _, f := range flags {
		if !strings.HasPrefix(f, "--") || f == "--" {
			return nil, fmt.Errorf("rsync flag %q is not a long option", f)
		}
		if err := checkTransportToken(f); err != nil {
			return nil, err
		}
		args = append(args, f)
	}
	eOpts := append(append([]string{}, t.opts...), t.spec.SSHOptions()...)
	// rsync parses -e again. Its parser represents a literal quote by doubling
	// it inside a quoted word (backslash escaping does not preserve it).
	for i, option := range eOpts {
		if strings.HasPrefix(option, `GlobalKnownHostsFile="`) {
			eOpts[i] = `"` + strings.ReplaceAll(option, `"`, `""`) + `"`
		}
	}
	args = append(args, "-e", "ssh "+strings.Join(eOpts, " "), "--")
	for _, src := range srcs {
		local, err := localOperand(src)
		if err != nil {
			return nil, err
		}
		args = append(args, local)
	}
	return append(args, t.operand(dest)), nil
}

// scpArgv is the scp argv: options, -P port, "--", local, host:dest. The
// caller validates dest. local becomes absolute.
func (t sshTransport) scpArgv(local, dest string) ([]string, error) {
	abs, err := localOperand(local)
	if err != nil {
		return nil, err
	}
	args := append([]string{}, t.opts...)
	if t.spec.Port != 0 {
		args = append(args, "-P", strconv.Itoa(t.spec.Port))
	}
	return append(args, "--", abs, t.operand(dest)), nil
}

// operand is the scp/rsync remote operand "[user@]host:path" (no port).
func (t sshTransport) operand(path string) string { return hostOperand(t.spec, path) }

// hostOperand renders "[user@]host:path"; an IPv6 host is bracketed.
func hostOperand(spec remote.HostSpec, path string) string {
	host := spec.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if spec.User != "" {
		host = spec.User + "@" + host
	}
	return host + ":" + path
}

// localOperand makes a relative p absolute, so scp and rsync never read it as
// a "host:path" operand or an option. A path that is already absolute (or
// starts with '/') is returned unchanged, so argv stays as it was on every OS.
// Empty paths, newlines and NUL are refused.
func localOperand(p string) (string, error) {
	if p == "" || strings.ContainsAny(p, "\n\x00") {
		return "", fmt.Errorf("local path %q is empty or contains a newline or NUL", p)
	}
	if strings.HasPrefix(p, "/") || filepath.IsAbs(p) {
		return p, nil
	}
	return filepath.Abs(p)
}
