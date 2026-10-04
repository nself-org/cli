// Package errs (exit codes) defines the canonical exit-code contract for the
// nSelf CLI. Every command path that can fail must map its failure to one of
// these codes so wrappers (CI runners, schedulers, ops scripts) can branch on
// outcome class without parsing stderr.
//
// The codes are intentionally small and stable. Adding a new class requires a
// release-plan note. Re-using an existing class is preferred over inventing a
// new one.
package errs

import "errors"

// Canonical exit codes. Must match the contract documented in
// .claude/docs/standards/exit-codes.md and surfaced via 'nself --help' in
// the EXIT CODES section.
const (
	// ExitOK indicates successful completion.
	ExitOK = 0

	// ExitUserError indicates the caller supplied invalid input: bad flag
	// value, missing required argument, malformed env, unknown subcommand.
	// The fix is on the user side.
	ExitUserError = 1

	// ExitInfraError indicates a downstream infrastructure failure: docker
	// daemon down, port collision, network unreachable, disk full, plugin
	// runtime crash. Retrying after fixing the host condition may succeed.
	ExitInfraError = 2

	// ExitAuthError indicates an authentication or authorization failure:
	// invalid license key, expired key, tier insufficient for the requested
	// plugin, missing OAuth credentials.
	ExitAuthError = 3

	// ExitDestructiveBlocked indicates a destructive action was refused by
	// the safety guards: missing --force on prod, force_local mismatch,
	// confirmation prompt declined.
	ExitDestructiveBlocked = 4
)

// ExitCodeFor classifies a top-level error into one of the canonical exit
// codes (contract:cli.exit-codes v1). It is the only mapping main uses.
//
// Classification order:
//  1. nil -> ExitOK.
//  2. An error exposing ExitCode() (ExitError, plugin ExitCodeError): that code.
//  3. A *CLIError whose registry entry has Exit != 0: that Exit (E400 has 0
//     and falls through).
//  4. The sentinel-table match with the highest class (auth > destructive >
//     infra > user when sentinels are joined): the Exit of its registry code.
//  5. Everything else: ExitUserError (1).
//
// Returning ExitOK is reserved for nil errors and never inferred from
// classification.
func ExitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}

	// An explicit ExitError (or anything else exposing ExitCode) wins over
	// classification: the command already decided its exit status.
	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}

	var ce *CLIError
	if errors.As(err, &ce) {
		if entry, ok := Registry[ce.Code]; ok && entry.Exit != 0 {
			return entry.Exit
		}
	}

	if code, ok := CodeForSentinel(err); ok {
		if entry, ok := Registry[code]; ok && entry.Exit != 0 {
			return entry.Exit
		}
	}

	return ExitUserError
}

// ErrDestructiveBlocked is the package-level sentinel that command glue can
// wrap with fmt.Errorf("...: %w", errs.ErrDestructiveBlocked) so that
// ExitCodeFor classifies the failure as exit code 4 without an import cycle
// through internal/confirm.
var ErrDestructiveBlocked = sentinel("destructive action blocked by safety gate")

// sentinel returns a plain error usable as an errors.Is target.
type sentinelErr string

func (e sentinelErr) Error() string { return string(e) }

func sentinel(s string) error { return sentinelErr(s) }
