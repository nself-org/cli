package commands

// imageswitch_guard.go — E512: while a postgres image switch is incomplete,
// every other command that changes the project is refused (EPIC D6).
//
// Purpose: a half-moved database must not meet `nself start`, `build` or
// `config set`. The check is one stat of .nself/image-switch/state.json inside
// a project; the registry is read only when a switch is incomplete.
// Inputs: the invoked command. Outputs: E512 naming --resume and --rollback, or nil.
// Constraints: `db image ...` itself is always allowed; read-class commands
// (status, logs, doctor) are never blocked; a command with no registry entry is
// not blocked.

import (
	"os"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/oplock/oplockcmd"
	"github.com/nself-org/cli/internal/postgres/imageswitch"
	"github.com/spf13/cobra"
)

// imageSwitchGuard returns E512 for a write, remote or destructive command run
// while an image switch is incomplete in the current project.
func imageSwitchGuard(cmd *cobra.Command) error {
	if cmd.Parent() == nil {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	root, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return nil
	}
	st, incomplete := imageswitch.Incomplete(root)
	if !incomplete {
		return nil
	}
	if key := invokedKey(cmd); key == "db image" || strings.HasPrefix(key, "db image ") {
		return nil
	}
	e, err := jsonEntryFor(cmd)
	if err != nil {
		return nil
	}
	switch side, _ := oplockcmd.Effective(cmd, e); side {
	case "write", "remote", "destructive":
	default:
		return nil
	}
	phase := string(st.Phase)
	if phase == "" {
		phase = "unreadable state"
	}
	return errs.New("E512", "nself "+invokedKey(cmd)+" is blocked: a postgres image switch is incomplete").
		WithWhy("the switch to a new volume stopped in phase \"" + phase + "\"; changing the project now could use the wrong database").
		WithFix("run `nself db image switch --resume` to finish it, or `nself db image switch --rollback` to go back to the old volume")
}

// firstErr returns the first non-nil error.
func firstErr(list ...error) error {
	for _, e := range list {
		if e != nil {
			return e
		}
	}
	return nil
}
