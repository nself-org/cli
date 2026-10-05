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
// (-a), no X11 forwarding (-x). scp has no such flags (its -T disables
// filename checking), so CopyTo drops them.
func CISSHFlags() []string { return []string{"-T", "-a", "-x"} }

// d4Option is one entry of the D4 -o set: a key with its fixed value, or, for
// the two entries that name a per-node value, a validator.
type d4Option struct {
	key, fixed string
}

// d4Options is the D4 set in its documented order. CIOptions emits it and
// checkOptions recognizes it from this one table.
var d4Options = []d4Option{
	{"BatchMode", "yes"}, {"ForwardAgent", "no"}, {"ForwardX11", "no"},
	{"ClearAllForwardings", "yes"}, {"PermitLocalCommand", "no"}, {"ControlMaster", "no"},
	{"ControlPath", "none"}, {"RemoteCommand", "none"}, {"RequestTTY", "no"},
	{"UpdateHostKeys", "no"}, {"CheckHostIP", "no"}, {"VerifyHostKeyDNS", "no"},
	{"StrictHostKeyChecking", "yes"}, {"UserKnownHostsFile", ""},
	{"GlobalKnownHostsFile", "/dev/null"}, {"HostKeyAlias", ""},
	{"ConnectTimeout", "10"}, {"ServerAliveInterval", "10"}, {"ServerAliveCountMax", "3"},
}

// literalPath accepts a pinned known_hosts path that ssh reads as written: a
// safeOptValue with no '%' token, no '$' (environment expansion) and no
// leading '~' (home expansion).
func literalPath(v string) bool {
	return safeOptValue(v) && !strings.ContainsAny(v, "%$") && v[0] != '~'
}

// matches reports whether kv ("Key=Value") is this D4 entry. The pinned file
// must be a plain path that is not /dev/null; the alias must be
// nself-ci-<valid node id>.
func (e d4Option) matches(kv string) bool {
	v, ok := strings.CutPrefix(kv, e.key+"=")
	if !ok {
		return false
	}
	switch e.key {
	case "UserKnownHostsFile":
		return literalPath(v) && v != "/dev/null" && v != "none"
	case "HostKeyAlias":
		id, ok := strings.CutPrefix(v, "nself-ci-")
		return ok && ValidateNodeID(id) == nil
	}
	return v == e.fixed
}

// CIOptions returns the D4 `-o` option set as flat "-o", "Key=Value" pairs, in
// the documented order. KnownHostsCommand=none is appended only for OpenSSH
// 8.5 or newer (older clients do not have the option and cannot be bypassed
// by it). nodeID must satisfy ValidateNodeID and pinnedFile must be the
// pinned known_hosts file; the caller supplies both.
func CIOptions(nodeID, pinnedFile string, v Version) []string {
	out := make([]string, 0, 2*len(d4Options)+2)
	for _, e := range d4Options {
		val := e.fixed
		switch e.key {
		case "UserKnownHostsFile":
			val = pinnedFile
		case "HostKeyAlias":
			val = "nself-ci-" + nodeID
		}
		out = append(out, "-o", e.key+"="+val)
	}
	if v.AtLeast(8, 5) {
		out = append(out, "-o", "KnownHostsCommand=none")
	}
	return out
}
