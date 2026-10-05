// Purpose: the ssh option sets CI uses to reach nodes, and local OpenSSH
//          version detection.
// Inputs:  a node id, the pinned known_hosts file, the local ssh version.
// Outputs: argv fragments: CISSHFlags (ssh only) and CIOptions (-o pairs for
//          ssh, scp and rsync -e).
// Constraints: -o options on the command line win over ssh_config, so the
//              operator's config cannot re-enable what is switched off here.

package remote

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is a parsed OpenSSH client version.
type Version struct{ Major, Minor int }

// AtLeast reports whether v >= major.minor.
func (v Version) AtLeast(major, minor int) bool {
	return v.Major > major || v.Major == major && v.Minor >= minor
}

var sshVersionRe = regexp.MustCompile(`OpenSSH_(?:for_Windows_)?(\d+)\.(\d+)`)

// ParseSSHVersion parses the output of `ssh -V` ("OpenSSH_9.6p1, ..." or the
// Windows build's "OpenSSH_for_Windows_9.5p1, ...").
func ParseSSHVersion(out string) (Version, error) {
	m := sshVersionRe.FindStringSubmatch(out)
	if m == nil {
		return Version{}, fmt.Errorf("cannot parse ssh version from %q", out)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return Version{Major: major, Minor: minor}, nil
}

// SSHVersion runs `ssh -V` and parses it.
func SSHVersion(ctx context.Context) (Version, error) {
	cmd, err := Command(ctx, "ssh", "-V")
	if err != nil {
		return Version{}, err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Version{}, fmt.Errorf("ssh -V: %w", err)
	}
	return ParseSSHVersion(string(out))
}

// CISSHFlags returns the ssh-only flags: no TTY (-T), no agent forwarding
// (-a), no X11 forwarding (-x). Never pass them to scp, where -T disables
// filename checking.
func CISSHFlags() []string { return []string{"-T", "-a", "-x"} }

// CIOptions returns the D4 `-o` option set as flat "-o", "Key=Value" pairs, in
// the documented order. KnownHostsCommand=none is appended only for OpenSSH
// 8.5 or newer (older clients do not have the option and cannot be bypassed
// by it). nodeID must satisfy ValidateNodeID and pinnedFile must be the
// pinned known_hosts file; the caller supplies both.
func CIOptions(nodeID, pinnedFile string, v Version) []string {
	kv := []string{
		"BatchMode=yes",
		"ForwardAgent=no",
		"ForwardX11=no",
		"ClearAllForwardings=yes",
		"PermitLocalCommand=no",
		"ControlMaster=no",
		"ControlPath=none",
		"RemoteCommand=none",
		"RequestTTY=no",
		"UpdateHostKeys=no",
		"CheckHostIP=no",
		"VerifyHostKeyDNS=no",
		"StrictHostKeyChecking=yes",
		"UserKnownHostsFile=" + pinnedFile,
		"GlobalKnownHostsFile=/dev/null",
		"HostKeyAlias=nself-ci-" + nodeID,
		"ConnectTimeout=10",
		"ServerAliveInterval=10",
		"ServerAliveCountMax=3",
	}
	if v.AtLeast(8, 5) {
		kv = append(kv, "KnownHostsCommand=none")
	}
	out := make([]string, 0, 2*len(kv))
	for _, o := range kv {
		out = append(out, "-o", o)
	}
	return out
}

// checkOptions refuses option elements that ssh would re-parse: a newline or
// NUL anywhere, and whitespace in the values of options that take file lists
// or aliases (ssh splits UserKnownHostsFile on whitespace, so a pinned file
// path with a space would silently name two files).
func checkOptions(opts []string) error {
	for _, o := range opts {
		if strings.ContainsAny(o, "\n\r\x00") {
			return fmt.Errorf("ssh option %q holds a newline or NUL", o)
		}
		for _, p := range []string{"UserKnownHostsFile=", "GlobalKnownHostsFile=", "HostKeyAlias=", "ProxyJump=", "ProxyCommand="} {
			if strings.HasPrefix(o, p) && strings.ContainsAny(o, " \t") {
				return fmt.Errorf("ssh option %q must not contain whitespace", o)
			}
		}
	}
	return nil
}
