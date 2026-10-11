package deploy

// Purpose: a real Ctrl-C for the v1.5 remote apply and rollback (G5). A SIGINT
//          or SIGTERM cancels the context the apply runs on; the remote children
//          (ssh, rsync) run in their own process group, so the terminal's SIGINT
//          does not kill the one in flight behind our back: the context does,
//          and only after the handler is installed.
// Inputs:  the caller's context.
// Outputs: a context cancelled on SIGINT/SIGTERM and its stop func.
//              SIGHUP (closed terminal or ssh session) is trapped on unix.
//              A cancel does not kill an in-flight ssh or rsync: it gets
//              stepGrace to finish, so the remote script it started is not
//              still running when the restore begins.
// Constraints: after the cancel, ApplyRemote's restore and the lock release run
//              on context.WithoutCancel bounded by a timeout, so a signal leaves
//              the host restored (before the first `up`) and unlocked. A second
//              signal is absorbed with one line saying it is still finishing the step or restoring: the restore is
//              bounded by restoreTimeout and the lock is released after it.

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"time"
)

// interruptOut is where the second-signal notice goes.
var interruptOut io.Writer = os.Stderr

// stepGrace bounds how long a cancelled ssh or rsync may keep running to finish
// its remote script before it is killed.
const stepGrace = 120 * time.Second

// WithInterrupt returns a context that is cancelled on the first SIGINT, SIGTERM
// or (unix) SIGHUP. A later signal does not kill the process: it prints one line
// that it is still finishing the step or restoring. Call stop when the operation ends.
func WithInterrupt(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, interruptSignals()...)
	done := make(chan struct{})
	go func() {
		select {
		case <-ch:
			cancel()
			_, _ = fmt.Fprintf(interruptOut, "\ninterrupted: finishing the current step (up to %s), then restoring if nothing was started yet\n", stepGrace)
		case <-done:
			return
		}
		told := false
		for {
			select {
			case <-ch:
				if !told {
					told = true
					_, _ = fmt.Fprintf(interruptOut, "\nsignal received again: still finishing the step or restoring (the restore is bounded by %s); the deploy lock is released after it; please wait\n", restoreTimeout)
				}
			case <-done:
				return
			}
		}
	}()
	var once sync.Once
	return ctx, func() {
		once.Do(func() { close(done); signal.Stop(ch); cancel() })
	}
}

// ownProcessGroup starts cmd in its own process group where the platform has
// one, so a terminal SIGINT aimed at our group does not reach it.
func ownProcessGroup(cmd *exec.Cmd) {
	detachProcessGroup(cmd)
	cmd.Cancel = func() error { return nil } // let the step finish; WaitDelay bounds it
	cmd.WaitDelay = stepGrace
}
