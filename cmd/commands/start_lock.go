package commands

// start_lock.go — `nself start --frozen` and the plain-start lock warning
// (contract:cli.lockfile v1, P7-LIVE-09, EPIC D10).
//
// Purpose: a CI or production start must run exactly the stack the committed
// nself.lock.yaml describes. --frozen fails with E455 on any mismatch (compose
// images, installed plugin versions and checksums, the CLI minor) or when the
// project has no lock; a plain start prints one warning and carries on.
// Inputs: the start command's flags and the project root.
// Outputs: nil, or an E455 error naming every mismatch. The warning goes to
// stderr, once per start.
// Constraints: runs as the command's PreRunE, before any docker call or
// automatic rebuild. Under --frozen it also disables the automatic rebuild and
// the AI auto-install, so the stack that was checked is the stack that starts. A project without a lock and without --frozen is silent.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/lock"

	"github.com/spf13/cobra"
)

// startLockPreRun is startCmd's PreRunE.
func startLockPreRun(cmd *cobra.Command, _ []string) error {
	frozen, _ := cmd.Flags().GetBool("frozen")
	root, err := projectRoot()
	if err != nil {
		if frozen {
			return err
		}
		return nil
	}
	if err := checkStartLock(root, frozen, cmd.ErrOrStderr()); err != nil {
		return err
	}
	if frozen {
		return freezeStart(cmd)
	}
	return nil
}

// freezeStart stops everything runStart does after this pre-run check from
// changing the stack that was just compared with the lock: the automatic
// rebuild (a full build that would re-render the compose and rewrite the lock)
// and the AI auto-install (it installs plugins, so it moves the plugin
// versions the lock pins). Both are switched off through the settings they
// already read: --skip-build and AI_AUTO_INSTALL=false.
func freezeStart(cmd *cobra.Command) error {
	if err := cmd.Flags().Set("skip-build", "true"); err != nil {
		return errs.New("E455", "cannot freeze the start: "+err.Error()).WithFix("report this as a bug")
	}
	if err := os.Setenv("AI_AUTO_INSTALL", "false"); err != nil {
		return errs.New("E455", "cannot freeze the start: "+err.Error()).WithFix("report this as a bug")
	}
	return nil
}

// checkStartLock compares the project with its lock. frozen turns a mismatch
// into E455; otherwise it warns on w.
func checkStartLock(root string, frozen bool, w io.Writer) error {
	locked, err := lock.Load(root)
	if errors.Is(err, lock.ErrNotFound) {
		if frozen {
			return errs.New("E455", "this project has no "+lock.FileName).
				WithWhy("--frozen starts only the stack a committed lock describes").
				WithFix("run nself build --write-lock, commit " + lock.FileName + ", then start with --frozen")
		}
		return nil
	}
	if err != nil {
		if frozen {
			return errs.New("E455", err.Error()).WithFix("run nself build to rewrite " + lock.FileName)
		}
		_, _ = fmt.Fprintf(w, "warning: %v\n", err)
		return nil
	}
	actual, err := build.CurrentLock(root)
	if err != nil {
		if frozen {
			return errs.New("E455", "cannot compare the stack with "+lock.FileName+": "+err.Error()).WithFix("run nself build, then start again")
		}
		_, _ = fmt.Fprintf(w, "warning: cannot compare the stack with %s: %v\n", lock.FileName, err)
		return nil
	}
	cmp := lock.Compare
	if frozen {
		cmp = lock.CompareStrict
		warnUnpinned(w, actual)
	}
	diff := cmp(*locked, actual)
	if len(diff) == 0 {
		return nil
	}
	lines := make([]string, len(diff))
	for i, m := range diff {
		lines[i] = "  " + m.String()
	}
	list := strings.Join(lines, "\n")
	if frozen {
		return errs.New("E455", fmt.Sprintf("the stack differs from %s in %d place(s):\n%s", lock.FileName, len(diff), list)).
			WithWhy("--frozen refuses to start a stack that is not the one the lock pins").
			WithFix("run nself build to regenerate the stack from the lock, or nself build --write-lock and commit the new lock to accept the change")
	}
	_, _ = fmt.Fprintf(w, "warning: the stack differs from %s (start --frozen would fail):\n%s\n", lock.FileName, list)
	return nil
}

// warnUnpinned prints one warning listing the services whose image is a tag
// only: a moved registry or local tag would not show as a mismatch for them.
func warnUnpinned(w io.Writer, actual lock.Lock) {
	if len(actual.Unpinned) == 0 {
		return
	}
	names := make([]string, 0, len(actual.Unpinned))
	for k := range actual.Unpinned {
		names = append(names, k)
	}
	sort.Strings(names)
	_, _ = fmt.Fprintf(w, "warning: %d service(s) are pinned by tag only, not by digest (a moved tag is not detected): %s\n", len(names), strings.Join(names, ", "))
}
