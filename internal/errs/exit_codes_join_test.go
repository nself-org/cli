package errs

import (
	"errors"
	"fmt"
	"testing"
)

// legacyExitCodeFor reproduces origin/main's ExitCodeFor classification for
// sentinels (the three hand lists and their order: auth, destructive_blocked,
// infra, else 1). It is the reference the joined-error precedence must match.
func legacyExitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	has := func(list ...error) bool {
		for _, target := range list {
			if errors.Is(err, target) {
				return true
			}
		}
		return false
	}
	switch {
	case has(ErrInvalidLicenseKey, ErrLicenseTierTooLow, ErrLicenseExpired, ErrLicenseNetworkUnavailable):
		return ExitAuthError
	case has(ErrDestructiveBlocked):
		return ExitDestructiveBlocked
	case has(ErrDockerNotRunning, ErrDockerNotInstalled, ErrComposeNotFound, ErrPortConflict,
		ErrDatabaseNotRunning, ErrServiceUnhealthy, ErrHealthTimeout, ErrServiceNotFound,
		ErrSSLGenerationFailed, ErrBackupFailed, ErrBackupNotFound, ErrBackupVerifyFailed,
		ErrBackupRestoreFailed, ErrWALArchiveFailed):
		return ExitInfraError
	}
	return ExitUserError
}

// legacyClassified are the sentinels whose class origin/main already knew.
var legacyClassified = []error{
	ErrInvalidLicenseKey, ErrLicenseTierTooLow, ErrLicenseExpired, ErrLicenseNetworkUnavailable,
	ErrDestructiveBlocked,
	ErrDockerNotRunning, ErrDockerNotInstalled, ErrComposeNotFound, ErrPortConflict,
	ErrDatabaseNotRunning, ErrServiceUnhealthy, ErrHealthTimeout, ErrServiceNotFound,
	ErrSSLGenerationFailed, ErrBackupFailed, ErrBackupNotFound, ErrBackupVerifyFailed,
	ErrBackupRestoreFailed, ErrWALArchiveFailed,
}

// TestExitCodeFor_JoinedPrecedence pins the named cases from the review:
// destructive and auth win over infra, whatever the join order, and the
// results equal origin/main's.
func TestExitCodeFor_JoinedPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"destructive+infra", errors.Join(ErrDestructiveBlocked, ErrDockerNotRunning), 4},
		{"infra+destructive", errors.Join(ErrDockerNotRunning, ErrDestructiveBlocked), 4},
		{"auth+infra", errors.Join(ErrDockerNotRunning, ErrLicenseExpired), 3},
		{"infra+auth", errors.Join(ErrLicenseExpired, ErrDockerNotRunning), 3},
		{"auth+destructive", errors.Join(ErrDestructiveBlocked, ErrInvalidLicenseKey), 3},
		{"wrapped destructive+infra", fmt.Errorf("x: %w", errors.Join(ErrDatabaseNotRunning, fmt.Errorf("y: %w", ErrDestructiveBlocked))), 4},
		{"infra+user", errors.Join(ErrInvalidDomain, ErrPortConflict), 2},
	} {
		if got := ExitCodeFor(tc.err); got != tc.want {
			t.Errorf("%s: ExitCodeFor = %d, want %d", tc.name, got, tc.want)
		}
		if got := legacyExitCodeFor(tc.err); got != tc.want {
			t.Errorf("%s: origin/main reference gives %d, test expectation %d is wrong", tc.name, got, tc.want)
		}
		if got := Describe(tc.err).ExitCode; got != tc.want {
			t.Errorf("%s: Describe exit_code = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestExitCodeFor_JoinedMatchesOrigin joins every pair of sentinels that
// origin/main already classified and requires the same exit status, then
// requires, for every pair in the table, that the higher class wins.
func TestExitCodeFor_JoinedMatchesOrigin(t *testing.T) {
	for _, a := range legacyClassified {
		for _, b := range legacyClassified {
			j := errors.Join(a, b)
			if got, want := ExitCodeFor(j), legacyExitCodeFor(j); got != want {
				t.Errorf("Join(%v, %v): ExitCodeFor = %d, origin/main = %d", a, b, got, want)
			}
		}
	}
	for _, a := range sentinelTable {
		for _, b := range sentinelTable {
			ea, eb := ExitCodeFor(a.Err), ExitCodeFor(b.Err)
			want := ea
			if classRank(eb) > classRank(ea) {
				want = eb
			}
			if got := ExitCodeFor(errors.Join(a.Err, b.Err)); got != want {
				t.Errorf("Join(%v, %v) = %d, want higher class %d", a.Err, b.Err, got, want)
			}
		}
	}
}
