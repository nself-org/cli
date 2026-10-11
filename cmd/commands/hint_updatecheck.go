package commands

// Update hint wiring (P7-ADOPT-08). The invocation decorator calls
// startUpdateCheck before a command body and afterCommand after it. Both are
// no-ops unless NSELF_UPDATE_CHECK=1 (internal/updatecheck.Allowed reads that
// variable first). The hint goes to the command's stderr, never stdout, and
// never in JSON mode.

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/updatecheck"
	"github.com/nself-org/cli/internal/version"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// updateCheckGuards builds the guards for one invocation. A variable so tests
// supply a clock, environment, TTY answer, cache path and server.
var updateCheckGuards = defaultUpdateCheckGuards

func defaultUpdateCheckGuards(cmd *cobra.Command) updatecheck.Guards {
	_, jsonOn, known := output.Invocation()
	return updatecheck.Guards{
		// An unrecorded invocation is treated as JSON mode: stay silent. So is a
		// legacy command asked for `--format json` or `--output json`: the
		// invocation record only counts those for envelope commands.
		JSON:    jsonOn || !known || jsonValuedFlag(cmd),
		TTY:     func() bool { return term.IsTerminal(int(os.Stderr.Fd())) },
		Env:     os.Getenv,
		Now:     time.Now,
		Path:    updatecheck.DefaultPath(),
		Current: version.GetVersion(),
	}
}

// jsonValuedFlag reports whether the command has a `format` or `output` flag
// (inherited ones included) whose value is "json", whatever the canon kind.
func jsonValuedFlag(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	for _, name := range []string{"format", "output"} {
		if f := cmd.Flags().Lookup(name); f != nil && strings.EqualFold(strings.TrimSpace(f.Value.String()), "json") {
			return true
		}
	}
	return false
}

// updateCheckStarted, when set by a test, receives the refresh's done channel.
var updateCheckStarted func(<-chan struct{})

// updateCheckSkipped reports commands that never get a hint: the update
// command itself already answers the question.
func updateCheckSkipped(cmd *cobra.Command) bool {
	k := invokedKey(cmd)
	return k == "update" || strings.HasPrefix(k, "update ")
}

// runWithUpdateHint runs body between startUpdateCheck and afterCommand. The
// invocation decorator calls it in place of calling the body directly.
func runWithUpdateHint(cmd *cobra.Command, args []string, body func(*cobra.Command, []string) error) error {
	early := startUpdateCheck(cmd)
	err := body(cmd, args)
	afterCommand(cmd, err, early)
	return err
}

// earlyHint is the hint decision startUpdateCheck takes before it starts a
// refresh. decided false leaves the decision to afterCommand; line is empty
// when no hint is due.
type earlyHint struct {
	decided bool
	line    string
}

// startUpdateCheck starts the background refresh when opted in and the cache
// is stale. It never blocks.
//
// When the cache is stale (this command is about to refresh it) the hint is
// decided first, from the cache as it is now (D-0361). So the refresh this
// command starts never feeds its own hint, and the hint never reads or writes
// the cache file while that refresh does: on Windows a rename over a file
// another handle has open fails, which dropped a due hint. A hint decided here
// is spent even if the command then fails; that costs at most one hint a day.
func startUpdateCheck(cmd *cobra.Command) earlyHint {
	if updateCheckSkipped(cmd) {
		return earlyHint{}
	}
	g := updateCheckGuards(cmd)
	var early earlyHint
	if updatecheck.Allowed(g) && g.Now().Sub(updatecheck.Load(g.Path).CheckedAt) >= updatecheck.Interval {
		early.decided = true
		early.line, _ = updatecheck.Hint(g)
	}
	if done := updatecheck.MaybeStartRefresh(g); done != nil && updateCheckStarted != nil {
		updateCheckStarted(done)
	}
	return early
}

// afterCommand prints the hint once, after a successful command: the one
// startUpdateCheck decided, or else one read from the cache now.
func afterCommand(cmd *cobra.Command, err error, early earlyHint) {
	if err != nil || updateCheckSkipped(cmd) {
		return
	}
	line := early.line
	if !early.decided {
		line, _ = updatecheck.Hint(updateCheckGuards(cmd))
	}
	if line != "" {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), line)
	}
}
