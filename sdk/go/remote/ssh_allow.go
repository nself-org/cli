// Purpose: the allowlist for Target.Options, the argv elements placed before
//          the destination of every ssh, scp and rsync -e call.
// Inputs:  Target.Options.
// Outputs: nil, or an error naming the refused element.
// Constraints: an allowlist. The D4 set (CISSHFlags + CIOptions) is accepted
//              only as one exact leading block; everything after it, or the
//              whole slice when there is no such block, may hold only the
//              options below. ssh keeps the first value it sees for an -o key,
//              so the D4 block always wins over what follows.

package remote

import (
	"fmt"
	"strings"
)

// sshOptionKeys are the -o keys a caller may set (lower case). None can run a
// command, change host-key policy, forward, multiplex or read another config.
var sshOptionKeys = map[string]bool{
	"connecttimeout": true, "serveraliveinterval": true, "serveralivecountmax": true,
	"port": true, "user": true, "identityfile": true, "identitiesonly": true,
	"batchmode": true, "compression": true, "loglevel": true,
	"connectionattempts": true, "addressfamily": true, "preferredauthentications": true,
}

var sshNumericKeys = map[string]bool{
	"connecttimeout": true, "serveraliveinterval": true, "serveralivecountmax": true,
	"port": true, "connectionattempts": true,
}

// checkOptions validates Target.Options.
func checkOptions(opts []string) error {
	for _, o := range opts {
		if strings.ContainsAny(o, "\n\r\x00") {
			return fmt.Errorf("ssh option %q holds a newline or NUL", o)
		}
	}
	_, rest := splitD4(opts)
	return checkCallerOptions(rest)
}

// splitD4 returns the leading D4 block (optional CISSHFlags, then every
// CIOptions pair in order, then the optional KnownHostsCommand=none) and the
// rest. If the block is not exactly that, d4 is nil and rest is all of opts.
func splitD4(opts []string) (d4, rest []string) {
	i := 0
	if len(opts) >= 3 && opts[0] == "-T" && opts[1] == "-a" && opts[2] == "-x" {
		i = 3
	}
	for _, e := range d4Options {
		if i+1 >= len(opts) || opts[i] != "-o" || !e.matches(opts[i+1]) {
			return nil, opts
		}
		i += 2
	}
	if i+1 < len(opts) && opts[i] == "-o" && opts[i+1] == "KnownHostsCommand=none" {
		i += 2
	}
	return opts[:i], opts[i:]
}

func checkCallerOptions(opts []string) error {
	for i := 0; i < len(opts); i++ {
		a := opts[i]
		next := func() (string, bool) {
			if i+1 < len(opts) {
				i++
				return opts[i], true
			}
			return "", false
		}
		switch {
		case a == "-4" || a == "-6" || a == "-q" || a == "-v":
		case a == "-o":
			v, ok := next()
			if !ok {
				return fmt.Errorf("ssh option -o has no value")
			}
			if err := checkSSHOption(v); err != nil {
				return err
			}
		case strings.HasPrefix(a, "-o") && len(a) > 2:
			if err := checkSSHOption(a[2:]); err != nil {
				return err
			}
		case a == "-i":
			v, ok := next()
			if !ok || !safeOptValue(v) {
				return fmt.Errorf("ssh option -i needs a plain path (got %q)", v)
			}
		case a == "-p":
			v, ok := next()
			if !ok || !isDigits(v) {
				return fmt.Errorf("ssh option -p needs a port number (got %q)", v)
			}
		default:
			return fmt.Errorf("ssh option %q is not allowed", a)
		}
	}
	return nil
}

// checkSSHOption validates the text after -o: `Key=Value`, `Key Value` or
// `Key = Value`, with Key (case-insensitive) in sshOptionKeys.
func checkSSHOption(s string) error {
	n := 0
	for n < len(s) && (s[n]|0x20 >= 'a' && s[n]|0x20 <= 'z') {
		n++
	}
	key := strings.ToLower(s[:n])
	rest := strings.TrimLeft(s[n:], " \t")
	if strings.HasPrefix(rest, "=") {
		rest = strings.TrimLeft(rest[1:], " \t")
	} else if rest == s[n:] {
		rest = "" // no separator: the key is followed by something else
	}
	if n == 0 || !sshOptionKeys[key] || !safeOptValue(rest) || (sshNumericKeys[key] && !isDigits(rest)) {
		return fmt.Errorf("ssh option %q is not allowed", s)
	}
	return nil
}

// safeOptValue accepts a non-empty value with no whitespace, quote,
// backslash, control character or leading '-'.
func safeOptValue(v string) bool {
	if v == "" || v[0] == '-' {
		return false
	}
	for _, r := range v {
		if r <= ' ' || r == 0x7f || r == '\'' || r == '"' || r == '\\' || r == '`' {
			return false
		}
	}
	return true
}

func isDigits(v string) bool {
	if v == "" || len(v) > 5 {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
