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
//              signal is not special: the restore is bounded by restoreTimeout.

import (
	"context"
	"os/exec"
	"os/signal"
	"time"
)

// stepGrace bounds how long a cancelled ssh or rsync may keep running to finish
// its remote script before it is killed.
const stepGrace = 120 * time.Second

// WithInterrupt returns a context that is cancelled on SIGINT or SIGTERM. Call
// stop when the operation ends to release the handler.
func WithInterrupt(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, interruptSignals()...)
}

// ownProcessGroup starts cmd in its own process group where the platform has
// one, so a terminal SIGINT aimed at our group does not reach it.
func ownProcessGroup(cmd *exec.Cmd) {
	detachProcessGroup(cmd)
	cmd.Cancel = func() error { return nil } // let the step finish; WaitDelay bounds it
	cmd.WaitDelay = stepGrace
}
