package commands

// Purpose: mask licence keys and JWTs in usage errors of a JSON invocation
// (P7-SURF-16); human usage text stays byte-identical.
// Inputs: the process argument list (invokedArgs) and the cobra error text.
// Outputs: the masked text.

import (
	"os"
	"strconv"
	"strings"
)

// invokedArgs is the argument list of this process; tests replace it.
var invokedArgs = func() []string { return os.Args[1:] }

// jsonInArgs reports whether the invocation asked for JSON output. A usage
// error fires while flags are still being parsed, so the flag value is not
// reliable yet and the argument list is read instead.
func jsonInArgs() bool {
	args := invokedArgs()
	on, format := false, ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			i = len(args)
		case a == "--json":
			on = true
		case strings.HasPrefix(a, "--json="):
			if v, err := strconv.ParseBool(a[len("--json="):]); err == nil {
				on = v
			}
		case strings.HasPrefix(a, "--format="):
			format = a[len("--format="):]
		case a == "--format" && i+1 < len(args):
			i++
			format = args[i]
		}
	}
	return on || format == "json"
}

// usageScrub masks licence keys and JWTs in a usage error, in a JSON
// invocation only: human usage text stays byte-identical.
func usageScrub(msg string) string {
	if !jsonInArgs() {
		return msg
	}
	return newScrubber().secrets(msg)
}
