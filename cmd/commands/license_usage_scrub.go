package commands

// Purpose: mask licence keys and JWTs in usage errors of a JSON invocation
// (P7-SURF-16); human usage text stays byte-identical.
// Inputs: the process argument list (invokedArgs) and the cobra error text.
// Outputs: the masked text.

import (
	"os"
	"strings"

	"github.com/nself-org/cli/internal/output"
)

// invokedArgs is the argument list of this process; tests replace it.
var invokedArgs = func() []string { return os.Args[1:] }

// jsonInArgs reports whether the invocation asked for JSON output. A usage
// error fires while flags are still being parsed, so the flag value is not
// reliable yet and the argument list is read instead.
func jsonInArgs() bool {
	args := invokedArgs()
	if output.JSONRequestedFromArgs(args) { // the same reading the error reporter uses
		return true
	}
	format := ""
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		switch {
		case strings.HasPrefix(args[i], "--format="):
			format = args[i][len("--format="):]
		case args[i] == "--format" && i+1 < len(args):
			i++
			format = args[i]
		}
	}
	return format == "json"
}

// usageScrub masks licence keys and JWTs in a usage error, in a JSON
// invocation only: human usage text stays byte-identical.
func usageScrub(msg string) string {
	if !jsonInArgs() {
		return msg
	}
	return newScrubber().secrets(msg)
}
