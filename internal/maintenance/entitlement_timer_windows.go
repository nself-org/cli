//go:build windows

package maintenance

// Purpose: keep the package building on Windows, where v1.5 self-hosting has no timer backend.
// Outputs: Ensure reports ErrNoBackend; Remove and RemoveAll find nothing; Status says no-backend.
// SPORT: cap:cli.entitlement-autorefresh.

// Ensure installs nothing on Windows.
func Ensure(string, string) (Result, error) { return Result{}, ErrNoBackend }

// Remove has nothing to remove on Windows.
func Remove(string) (RemoveResult, error) { return RemoveResult{}, nil }

// RemoveAll has nothing to remove on Windows.
func RemoveAll() (RemoveResult, error) { return RemoveResult{}, nil }

// Status reports that no backend exists on Windows.
func Status(string) TimerStatus { return TimerStatus{Reason: ReasonNoBackend} }
