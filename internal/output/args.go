package output

import "strconv"

// JSONRequestedFromArgs reports whether raw command-line arguments ask for JSON
// mode: `--json`, `--json=true` and `--json=1` are true, `--json=false` is
// false. Scanning stops at a `--` terminator; the last occurrence wins, as it
// does for a pflag bool. An unparsable value counts as false.
//
// It exists for failures that happen before cobra resolves a command (an
// unknown command or flag), where Invocation has nothing recorded yet.
func JSONRequestedFromArgs(args []string) bool {
	on := false
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--json" {
			on = true
			continue
		}
		const prefix = "--json="
		if len(a) > len(prefix) && a[:len(prefix)] == prefix {
			v, err := strconv.ParseBool(a[len(prefix):])
			on = err == nil && v
		}
	}
	return on
}
