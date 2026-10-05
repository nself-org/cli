package reconcile

// confirm.go — the confirmation rules of a plan (EPIC P7-LIVE D4).
//
// Purpose: decide whether Apply may proceed. A prod-class env needs --yes or an
// interactive yes for any non-empty plan; overwriting or removing a hand-edited
// generated file needs --force or an interactive yes in any env.
// Inputs: the plan, the operator's ApplyOptions, the compat mode (v1.5 or not)
// and a writer for notices.
// Outputs: nil to proceed, or an E403 error (exit 4) naming what is missing.
// Constraints: the refusals are v1.5 behaviour (ADR 0021). In v1.4 mode a plan
// that would have been refused prints a one-line notice and proceeds, as build
// always did. Non-interactive sessions (Interactive nil) are never prompted.

import (
	"fmt"
	"io"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// ConfirmFlagHint is the flag a non-interactive caller adds to proceed.
const ConfirmFlagHint = "--yes"

// Confirm applies the D4 rules to p.
func Confirm(p Plan, opt ApplyOptions, v15 bool, stderr io.Writer) error {
	if p.Empty {
		return nil
	}
	needYes := p.RequiresConfirmation && !opt.Yes
	hand := HandEditedPaths(p)
	needForce := len(hand) > 0 && !opt.Force
	if !needYes && !needForce {
		return nil
	}
	if !v15 {
		// v1.4 mode (the compat.V15 gate is at the caller, apply.go): notice, then proceed.
		if stderr != nil {
			_, _ = fmt.Fprintf(stderr, "nself: this is a %s change on a %s env; v1.5 will require %s (NSELF_V15=1 shows it now)\n",
				p.Command, p.EnvClass, ConfirmFlagHint)
		}
		return nil
	}
	if opt.Interactive != nil {
		if opt.Interactive(prompt(p, needYes, needForce, hand)) {
			return nil
		}
		return refusal("declined at the prompt", needYes, needForce, hand)
	}
	return refusal("this session is not interactive", needYes, needForce, hand)
}

// prompt is the question shown at a terminal; a destructive plan is worded
// strongly and lists its reasons.
func prompt(p Plan, needYes, needForce bool, hand []string) string {
	var sb strings.Builder
	if p.Destructive {
		sb.WriteString("This plan is DESTRUCTIVE:\n")
		for _, r := range p.DestructiveReasons {
			fmt.Fprintf(&sb, "  - %s\n", r)
		}
	}
	switch {
	case needYes && needForce:
		fmt.Fprintf(&sb, "Apply to a %s env and overwrite %d hand-edited file(s)?", p.EnvClass, len(hand))
	case needForce:
		fmt.Fprintf(&sb, "Overwrite %d hand-edited file(s)?", len(hand))
	default:
		fmt.Fprintf(&sb, "Apply this change to a %s env (%s)?", p.EnvClass, p.Env)
	}
	return sb.String()
}

// refusal builds the E403 error: what is missing and the flag that supplies it.
func refusal(why string, needYes, needForce bool, hand []string) error {
	var flags []string
	var what []string
	if needYes {
		flags = append(flags, "--yes")
		what = append(what, "a prod-class change needs confirmation")
	}
	if needForce {
		flags = append(flags, "--force")
		what = append(what, "hand-edited generated files would be overwritten: "+strings.Join(hand, ", "))
	}
	return errs.New("E403", fmt.Sprintf("refusing to apply: %s (%s)", strings.Join(what, "; "), why)).
		WithWhy("a safety gate needs an explicit confirmation before this change is applied").
		WithFix(fmt.Sprintf("review the plan with nself build --plan, then pass %s", strings.Join(flags, " ")))
}
