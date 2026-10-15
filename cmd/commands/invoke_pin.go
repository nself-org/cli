package commands

// invoke_pin.go — the child side of the machine invocation path pin.
//
// Purpose: a command run by internal/invoke (MCP, HTTP) must be the command
//   the invoker classified. The invoker sets NSELF_INVOKE_PATH to the
//   registry path it checked for exposure, side effect and confirmation; the
//   child resolves its own argv with the same cobra lookup Execute uses and
//   refuses (E421) when the two differ. This closes the bare-flag lookup
//   shift on flag-less plugin nodes and a registry snapshot that went stale
//   (plugin upgrade or binary swap) after the server started.
//   A matching path is also checked against the child's exposure attributes
//   before execution (invoke_exposure.go).
// Inputs:  NSELF_INVOKE_PATH, the prepared root command and its argv.
// Outputs: nil, or an E421 error naming a path mismatch or exposure refusal.
// Constraints: the variable is removed from this process's environment before
//   the check, so commands this child spawns never inherit the pin. Unset
//   means no pin (every non-machine invocation).
// SPORT: CLI-CMD-MCP-001

import (
	"os"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/spf13/cobra"
)

// checkInvokePin refuses a machine-invoked child whose argv resolves to a
// command other than the one the invoker classified.
func checkInvokePin(root *cobra.Command, argv []string) error {
	want, ok := os.LookupEnv(invoke.PathEnv)
	if !ok {
		return nil
	}
	_ = os.Unsetenv(invoke.PathEnv)
	got := ""
	if c, _, _ := root.Find(argv); c != nil {
		got = strings.TrimSpace(strings.TrimPrefix(c.CommandPath(), root.Name()))
	}
	if want = strings.TrimSpace(want); want != "" && got == want {
		return checkInvokeExposure(root, argv, got)
	}
	return errs.New("E421", "command "+quoteInvokePath(want)+" is not exposed").
		WithWhy("the child resolved its arguments to " + quoteInvokePath(got) + ", not the command the invoker checked; restart the server after a plugin or binary change")
}

// quoteInvokePath quotes a command path for a message, bounded in length.
func quoteInvokePath(p string) string {
	if r := []rune(p); len(r) > 96 {
		p = string(r[:96])
	}
	return `"` + p + `"`
}
