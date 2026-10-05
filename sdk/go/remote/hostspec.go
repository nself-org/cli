// Purpose: the one deploy host grammar. ParseHostSpec turns "[user@]host[:port]"
//          (and the legacy "[user@]host:/abs/path") into a HostSpec whose
//          argv forms are safe to hand to ssh, scp and rsync.
// Inputs:  a host string from a CLI flag, an inventory file or an environment
//          variable: untrusted.
// Outputs: HostSpec{User, Host, Port, LegacyPath}, or *HostSpecError (callers
//          map it to E484). HostSpec renders String (canonical), Dest,
//          SSHArgs, SSHOptions and Target.
// Constraints: a hand-written scanner over allowlists, no regular expression.
//              user ^[a-z_][a-z0-9_.-]{0,31}$; host a dotted name of labels
//              [A-Za-z0-9_-] (at most 253 bytes, no empty label, never a
//              leading '-'; '_' keeps ssh-config aliases working), a dotted
//              IPv4 address (which is such a name) or a bracketed IPv6
//              literal; port 1-65535 in decimal without sign or leading zero;
//              legacy path passes ValidateCopyPath. Whitespace, control and
//              non-ASCII bytes, shell metacharacters and a second '@' or ':'
//              fall outside every allowlist. Nothing here starts a process or
//              calls a shell.

package remote

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	maxUserLen = 32
	maxHostLen = 253
	maxPort    = 65535
	maxEcho    = 128 // longest Input a HostSpecError keeps
)

// HostSpec is a parsed deploy host. Port 0 means "not given" (ssh's default).
// Host holds an IPv6 literal without its brackets. LegacyPath is set only by
// the legacy "host:/abs/path" form and never together with Port.
type HostSpec struct {
	User       string
	Host       string
	Port       int
	LegacyPath string
}

// HostSpecError is the error ParseHostSpec returns. Input is the offending
// text (cut to 128 bytes), Reason says which rule it broke.
type HostSpecError struct {
	Input  string
	Reason string
}

func (e *HostSpecError) Error() string {
	return fmt.Sprintf("invalid host %q: %s", e.Input, e.Reason)
}

func specErr(in, format string, a ...any) error {
	if len(in) > maxEcho {
		in = in[:maxEcho] + "..."
	}
	return &HostSpecError{Input: in, Reason: fmt.Sprintf(format, a...)}
}

// ParseHostSpec parses s. The accepted forms are `host`, `user@host`,
// `[user@]host:port`, `[user@][ipv6]`, `[user@][ipv6]:port` and the legacy
// `[user@]host:/abs/path` (also with a bracketed IPv6 host).
func ParseHostSpec(s string) (HostSpec, error) {
	var h HostSpec
	rest := s
	if user, after, ok := strings.Cut(s, "@"); ok {
		if !validUser(user) {
			return HostSpec{}, specErr(s, "user must match [a-z_][a-z0-9_.-]{0,31}")
		}
		h.User, rest = user, after
	}
	var tail string
	var hasTail bool
	if strings.HasPrefix(rest, "[") {
		inner, after, ok := strings.Cut(rest[1:], "]")
		if !ok {
			return HostSpec{}, specErr(s, "unterminated '['")
		}
		if ip := net.ParseIP(inner); ip == nil || !strings.Contains(inner, ":") {
			return HostSpec{}, specErr(s, "brackets hold an IPv6 address only")
		}
		h.Host = inner
		if after != "" {
			if after[0] != ':' {
				return HostSpec{}, specErr(s, "text after ']' must be :port or :/path")
			}
			tail, hasTail = after[1:], true
		}
	} else {
		h.Host, tail, hasTail = strings.Cut(rest, ":")
		if err := checkName(h.Host); err != nil {
			return HostSpec{}, specErr(s, "%v", err)
		}
	}
	if !hasTail {
		return h, nil
	}
	if strings.HasPrefix(tail, "/") {
		if err := ValidateCopyPath(tail); err != nil {
			return HostSpec{}, specErr(s, "legacy path: %v", err)
		}
		h.LegacyPath = tail
		return h, nil
	}
	port, err := parsePort(tail)
	if err != nil {
		return HostSpec{}, specErr(s, "%v", err)
	}
	h.Port = port
	return h, nil
}

// validUser reports whether u matches [a-z_][a-z0-9_.-]{0,31}.
func validUser(u string) bool {
	if u == "" || len(u) > maxUserLen {
		return false
	}
	for i := 0; i < len(u); i++ {
		c := u[i]
		if c >= 'a' && c <= 'z' || c == '_' {
			continue
		}
		if i > 0 && (c >= '0' && c <= '9' || c == '.' || c == '-') {
			continue
		}
		return false
	}
	return true
}

// checkName checks a dotted host name: at most 253 bytes, labels of
// [A-Za-z0-9_-], no empty label, no leading '-'.
func checkName(h string) error {
	if len(h) > maxHostLen {
		return fmt.Errorf("host is longer than %d bytes", maxHostLen)
	}
	if strings.HasPrefix(h, "-") {
		return fmt.Errorf("host must not start with '-'")
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" {
			return fmt.Errorf("host is empty or has an empty label")
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
				return fmt.Errorf("host holds a byte outside [A-Za-z0-9_.-]")
			}
		}
	}
	return nil
}

// parsePort reads a port: 1-65535, decimal digits only, no leading zero.
func parsePort(t string) (int, error) {
	if !isDigits(t) { // 1 to 5 ASCII digits: no sign, no space, no overflow
		return 0, fmt.Errorf("port must be 1 to 5 decimal digits")
	}
	if t[0] == '0' {
		return 0, fmt.Errorf("port must not start with '0'")
	}
	n, _ := strconv.Atoi(t) // cannot fail: isDigits bounds t
	if n > maxPort {
		return 0, fmt.Errorf("port must be at most %d", maxPort)
	}
	return n, nil
}

// String is the canonical form [user@]host[:port], an IPv6 host in brackets.
// The legacy path is not part of it (that form is being removed): parsing
// String() again yields the same String().
func (h HostSpec) String() string {
	host := h.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if h.User != "" {
		host = h.User + "@" + host
	}
	if h.Port > 0 {
		host += ":" + strconv.Itoa(h.Port)
	}
	return host
}

// Dest is the ssh destination "[user@]host" without a port or path, an IPv6
// host bare: the form Target.Dest takes (ValidateDest accepts it; CopyTo and
// Rsync wrap the IPv6 host in brackets for scp and rsync operands).
func (h HostSpec) Dest() string {
	if h.User != "" {
		return h.User + "@" + h.Host
	}
	return h.Host
}

// SSHOptions are the options that carry the port: `-p N`, or nothing when no
// port was given. They never hold the destination. They go into
// Target.Options (after the D4 block): ssh and rsync -e read `-p N`, and scp's
// option translation turns it into `-P N`.
func (h HostSpec) SSHOptions() []string {
	if h.Port > 0 {
		return []string{"-p", strconv.Itoa(h.Port)}
	}
	return nil
}

// SSHArgs is the argv tail for ssh: SSHOptions, then "--", then Dest. The
// "--" ends option parsing, so no destination can be read as an option.
func (h HostSpec) SSHArgs() []string {
	return append(h.SSHOptions(), "--", h.Dest())
}

// Target returns the Target for h. opts are the ssh options placed before the
// port (the CI set, `-i KEY`, ...); nil means none, never BaseOptions, so the
// result's Options is always non-nil and checkOptions judges all of it.
func (h HostSpec) Target(opts []string) Target {
	return Target{Dest: h.Dest(), Options: append(append([]string{}, opts...), h.SSHOptions()...)}
}

// Validate reports whether h is what ParseHostSpec would produce, for a
// HostSpec built by hand: it parses h again and compares.
func (h HostSpec) Validate() error {
	probe := h.String()
	if h.LegacyPath != "" {
		probe = HostSpec{User: h.User, Host: h.Host}.String() + ":" + h.LegacyPath
	}
	got, err := ParseHostSpec(probe)
	if err != nil {
		return err
	}
	if got != h {
		return specErr(probe, "not a parsed host spec (Port with LegacyPath, or a negative Port)")
	}
	return nil
}
