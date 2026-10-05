// Purpose: validation of the values that reach ssh, scp and rsync as operands.
// Inputs:  remote paths, destinations, host aliases and node ids from callers
//          (CLI flags, inventory files, environment variables, node records).
// Outputs: nil when the value is safe to pass, otherwise an error naming it.
// Constraints: allowlists, not blacklists. Every check runs before any exec.

package remote

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// RemotePathRe allows safe remote path characters: alphanumeric, slash,
// hyphen, underscore, dot. Anything else (';', '$', '`', '|', '&', spaces,
// etc.) is rejected, since the value is later embedded directly into a
// shell command string executed on a remote host.
var RemotePathRe = regexp.MustCompile(`^[a-zA-Z0-9/_.-]+$`)

// ValidateRemotePath returns an error when path is non-empty and contains
// characters outside RemotePathRe's allowed charset. An empty path is
// treated as valid here — callers that require a non-empty path (e.g. a
// remote deploy target) must check that separately; ssh.go's DeployViaSsh
// already falls back to /tmp when the recovered remote path is empty.
//
// Beyond the charset it refuses a leading "-" (an option for the remote tool)
// and any ".." segment (a directory escape). The charset check also refuses
// NUL, newline and every shell metacharacter.
func ValidateRemotePath(path string) error {
	if path == "" {
		return nil
	}
	if !RemotePathRe.MatchString(path) {
		return fmt.Errorf("remote path contains unsafe characters (got %q): only [a-zA-Z0-9/_.-] allowed", path)
	}
	if path[0] == '-' {
		return fmt.Errorf("remote path must not start with '-' (got %q)", path)
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." {
			return fmt.Errorf("remote path must not contain '..' segments (got %q)", path)
		}
	}
	return nil
}

// maxHostToken bounds a destination or alias (DNS name 253 + user + '@').
const maxHostToken = 320

// ValidateAlias accepts a host alias, a hostname, an IP literal or a
// user@host destination: [A-Za-z0-9._@:+-], at most 320 bytes, not starting
// with '-'. The set holds no whitespace, quote, glob or shell metacharacter.
func ValidateAlias(s string) error {
	if s == "" {
		return fmt.Errorf("host is empty")
	}
	if len(s) > maxHostToken {
		return fmt.Errorf("host is longer than %d bytes", maxHostToken)
	}
	if s[0] == '-' {
		return fmt.Errorf("host %q must not start with '-'", s)
	}
	for i := 0; i < len(s); i++ {
		if !aliasByte(s[i]) {
			return fmt.Errorf("host %q contains an unsafe character at byte %d", s, i)
		}
	}
	return nil
}

func aliasByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return strings.IndexByte("._@:+-", c) >= 0
}

// ValidateDest checks an ssh/scp destination ("host" or "user@host"). It is
// ValidateAlias under the name callers of Run, CopyTo and Rsync expect.
func ValidateDest(dest string) error { return ValidateAlias(dest) }

// ValidateNodeID accepts the id placed in HostKeyAlias=nself-ci-<id>:
// [A-Za-z0-9._-], 1..128 bytes, not starting with '-'.
func ValidateNodeID(id string) error {
	if id == "" || len(id) > 128 || id[0] == '-' {
		return fmt.Errorf("node id %q is empty, too long or starts with '-'", id)
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return fmt.Errorf("node id %q contains an unsafe character at byte %d", id, i)
		}
	}
	return nil
}

// ValidateLegacyDest is the minimal destination check Compat targets get: only
// what could turn the destination into an option or split it.
func ValidateLegacyDest(dest string) error {
	if dest == "" {
		return fmt.Errorf("destination is empty")
	}
	if dest[0] == '-' {
		return fmt.Errorf("destination %q must not start with '-'", dest)
	}
	for i := 0; i < len(dest); i++ {
		if dest[i] <= ' ' || dest[i] == 0x7f {
			return fmt.Errorf("destination %q contains whitespace or a control character", dest)
		}
	}
	return nil
}

// isIPLiteral reports whether s parses as an IP address; pinned host keys are
// keyed by alias and never by address.
func isIPLiteral(s string) bool { return net.ParseIP(s) != nil }
