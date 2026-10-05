// Purpose: the allowlist for caller-supplied rsync options.
// Inputs:  the args slice a caller hands to Rsync.
// Outputs: nil, or an error naming the refused element.
// Constraints: an allowlist, never a deny-list. Every accepted element is
//              self-contained: a single-dash bundle of letters that take no
//              value, a long flag that takes no value, or a long flag with
//              its value attached by '='. So no caller element can consume
//              the next argv element (the sdk's "--", the source or the
//              destination), replace the transport, read a local file or run
//              a command.

package remote

import (
	"fmt"
	"regexp"
	"strings"
)

// rsyncShortLetters are the short flags that take no value: -a -r -l -p -t -g
// -o -D -z -v -c -h -n -u -i. Anything else (notably -e -M -T -B -f -s) is
// refused, including inside a bundle.
const rsyncShortLetters = "arlptgoDzvchnui"

var rsyncLongBare = map[string]bool{
	"--delete": true, "--checksum": true, "--partial": true, "--progress": true,
	"--compress": true, "--archive": true, "--recursive": true, "--times": true,
	"--perms": true, "--dry-run": true, "--itemize-changes": true, "--stats": true,
	"--human-readable": true, "--delete-after": true, "--mkpath": true,
}

// rsyncLongValue maps a `--name=value` flag to whether its value is a free
// pattern (true) or must match rsyncSimpleValue (false). Patterns travel over
// the rsync protocol, never the remote command line.
var rsyncLongValue = map[string]bool{
	"--exclude": true, "--include": true,
	"--chmod": false, "--chown": false, "--timeout": false, "--bwlimit": false, "--info": false,
}

var rsyncSimpleValue = regexp.MustCompile(`^[A-Za-z0-9,:=+._@-]+$`)

// checkRsyncArgs refuses every caller rsync element outside the allowlist.
func checkRsyncArgs(args []string) error {
	for _, a := range args {
		if err := checkRsyncArg(a); err != nil {
			return err
		}
	}
	return nil
}

func checkRsyncArg(a string) error {
	bad := func() error { return fmt.Errorf("rsync option %q is not allowed", a) }
	if strings.ContainsAny(a, "\n\r\x00") || len(a) < 2 || a[0] != '-' {
		return bad()
	}
	if strings.HasPrefix(a, "--") {
		name, val, hasVal := strings.Cut(a, "=")
		if !hasVal {
			if rsyncLongBare[name] {
				return nil
			}
			return bad()
		}
		pattern, ok := rsyncLongValue[name]
		if !ok || (!pattern && !rsyncSimpleValue.MatchString(val)) {
			return bad()
		}
		return nil
	}
	for _, r := range a[1:] {
		if !strings.ContainsRune(rsyncShortLetters, r) {
			return bad()
		}
	}
	return nil
}
