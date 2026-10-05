// Package oplockcmd binds the project operation lock to a cobra command.
//
// Purpose: keep internal/oplock free of cobra and the command registry. The
// invocation decorator (cmd/commands/invocation.go) wraps every command body
// with Wrap; Wrap decides, from the command's registry entry, whether the body
// runs under the lock.
//
// Inputs: an entry lookup (the decorator's registry resolver, a variable in
// tests) and the original RunE.
//
// Outputs: a RunE that takes the lock through oplock.Guard before the body,
// stores the Lock in the command context (oplock.FromContext, for early
// release) and releases it by defer on every return and panic.
//
// Constraints:
//   - A lock is taken only inside a project (config.FindNSelfRoot of cwd) and
//     only for a command with a registry entry; the registry is not consulted
//     outside a project, so a run there pays no canon parse.
//   - The class is the registry class after flag escalation: a given flag only
//     raises the side effect and replaces the output kind (EPIC D5/D6).
package oplockcmd

import (
	"context"
	"os"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/oplock"
	"github.com/spf13/cobra"
)

// EntryFunc resolves the registry entry of a command.
type EntryFunc func(*cobra.Command) (*cmdregistry.Command, error)

// Wrap returns orig run under the project operation lock policy.
func Wrap(entry EntryFunc, orig func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx, release, err := take(cmd, entry)
		if err != nil {
			return err
		}
		defer release()
		cmd.SetContext(ctx)
		return orig(cmd, args)
	}
}

// take resolves the project and the effective class and asks oplock.Guard.
func take(cmd *cobra.Command, entry EntryFunc) (context.Context, func(), error) {
	ctx, none := cmd.Context(), func() {}
	if ctx == nil {
		ctx = context.Background()
	}
	cwd, err := os.Getwd()
	if err != nil || cmd.Parent() == nil {
		return ctx, none, nil
	}
	dir, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return ctx, none, nil
	}
	e, err := entry(cmd)
	if err != nil {
		return ctx, none, nil
	}
	side, out := Effective(cmd, e)
	return oplock.Guard(ctx, oplock.Request{Dir: dir, Command: cmd.CommandPath(), SideEffect: side, Output: out})
}

// Effective returns the side effect and output kind of e after the flags given
// on cmd: a flag only escalates the side effect and replaces the output kind.
func Effective(cmd *cobra.Command, e *cmdregistry.Command) (side, out string) {
	side, out = e.SideEffect, e.Output
	for _, f := range e.Flags {
		pf := cmd.Flags().Lookup(f.Name)
		if pf == nil || !pf.Changed || (pf.Value.Type() == "bool" && pf.Value.String() != "true") {
			continue
		}
		if f.SideEffect != nil && canon.Rank(*f.SideEffect) > canon.Rank(side) {
			side = *f.SideEffect
		}
		if f.Output != nil {
			out = *f.Output
		}
	}
	return side, out
}
