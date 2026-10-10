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

func defaultUpdateCheckGuards(*cobra.Command) updatecheck.Guards {
	_, jsonOn, known := output.Invocation()
	return updatecheck.Guards{
		// An unrecorded invocation is treated as JSON mode: stay silent.
		JSON:    jsonOn || !known,
		TTY:     func() bool { return term.IsTerminal(int(os.Stderr.Fd())) },
		Env:     os.Getenv,
		Now:     time.Now,
		Path:    updatecheck.DefaultPath(),
		Current: version.GetVersion(),
	}
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
	startUpdateCheck(cmd)
	err := body(cmd, args)
	afterCommand(cmd, err)
	return err
}

// startUpdateCheck starts the background refresh when opted in and the cache
// is stale. It never blocks.
func startUpdateCheck(cmd *cobra.Command) {
	if updateCheckSkipped(cmd) {
		return
	}
	if done := updatecheck.MaybeStartRefresh(updateCheckGuards(cmd)); done != nil && updateCheckStarted != nil {
		updateCheckStarted(done)
	}
}

// afterCommand prints the hint once, after a successful command.
func afterCommand(cmd *cobra.Command, err error) {
	if err != nil || updateCheckSkipped(cmd) {
		return
	}
	if line, ok := updatecheck.Hint(updateCheckGuards(cmd)); ok {
		fmt.Fprintln(cmd.ErrOrStderr(), line)
	}
}
