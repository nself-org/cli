// Package license — grace.go implements the license grace period state machine
// and degradation mode enforcement.
//
// States: valid -> grace_soft -> grace_hard -> grace_post_expiry -> expired -> revoked
// Grace periods:
//   - <24h offline: proceed silently (valid)
//   - 24h-7d offline: WARNING banner (grace_soft)
//   - >7d offline: read-only degraded mode (grace_hard)
//   - License expired (server-reported expires_at) but <30d since expiry:
//     proceed with a warning, writes still allowed (grace_post_expiry)
//   - License expired and >=30d since expiry: refuse to start (expired)
//   - Revoked license: refuse to start (revoked)
package license

import (
	"fmt"
	"time"
)

// GraceState represents the license grace period state.
type GraceState string

const (
	// GraceValid means the license is validated and current.
	GraceValid GraceState = "valid"
	// GraceSoft means the cache is 24h-7d old; show warning banner.
	GraceSoft GraceState = "grace_soft"
	// GraceHard means the cache is >7d old; paid plugin writes are refused.
	GraceHard GraceState = "grace_hard"
	// GracePostExpiry means the license's server-reported expiry has passed
	// but the license is within the PostExpiryGraceWindow (30 days); paid
	// plugins keep working with a warning.
	GracePostExpiry GraceState = "grace_post_expiry"
	// GraceExpired means the license has expired and the post-expiry grace
	// window (PostExpiryGraceWindow) has also elapsed; paid plugins are
	// dormant.
	GraceExpired GraceState = "expired"
	// GraceRevoked means the license was explicitly revoked.
	GraceRevoked GraceState = "revoked"
)

// GraceSoftThreshold is when the soft grace warning starts (24 hours).
const GraceSoftThreshold = 24 * time.Hour

// GraceHardThreshold is when hard degradation begins (7 days).
// Configurable via LICENSE_GRACE_DAYS env var.
const GraceHardThreshold = 7 * 24 * time.Hour

// GraceCheckResult contains the outcome of a grace period check.
type GraceCheckResult struct {
	State        GraceState
	CacheAge     time.Duration
	ExpiresAt    time.Time
	Tier         string
	Message      string
	CanProceed   bool
	WriteAllowed bool
}

// DetermineGraceState evaluates the current grace state from a cache entry.
// If entry is nil, returns GraceExpired (no cache means no validation).
func DetermineGraceState(entry *CacheEntry) GraceCheckResult {
	if entry == nil {
		return GraceCheckResult{
			State:        GraceExpired,
			Message:      "No license cache found. Run 'nself license validate' to validate your license.",
			CanProceed:   false,
			WriteAllowed: false,
		}
	}

	now := time.Now()
	cacheAge := entry.CacheAge()
	expiresAt := time.Unix(entry.ExpiresAt, 0)

	// Check if the license itself has expired (server-reported expiry).
	// Commercial promise (Bundle License §4 / licensing.mdx / pricing FAQ):
	// paid plugins keep working for PostExpiryGraceWindow (30 days) after
	// expiry before going dormant. See P6-E12-W4-S4-T2.
	if entry.ExpiresAt > 0 && now.After(expiresAt) {
		return postExpiryGraceState(entry, expiresAt, cacheAge, now)
	}

	return cacheFreshnessGraceState(entry, expiresAt, cacheAge)
}

// cacheFreshnessGraceState evaluates the grace state for a non-expired
// license based on how long ago the cache was last successfully fetched.
// Split out of DetermineGraceState to keep that function under the repo's
// 50-line cap.
func cacheFreshnessGraceState(entry *CacheEntry, expiresAt time.Time, cacheAge time.Duration) GraceCheckResult {
	switch {
	case cacheAge < GraceSoftThreshold:
		return GraceCheckResult{
			State:        GraceValid,
			CacheAge:     cacheAge,
			ExpiresAt:    expiresAt,
			Tier:         entry.Tier,
			CanProceed:   true,
			WriteAllowed: true,
		}
	case cacheAge < GraceHardThreshold:
		return GraceCheckResult{
			State:     GraceSoft,
			CacheAge:  cacheAge,
			ExpiresAt: expiresAt,
			Tier:      entry.Tier,
			Message: fmt.Sprintf(
				"License validation is %s old. Connect to the internet to refresh.\n"+
					"Offline grace period expires in %s.",
				formatDuration(cacheAge),
				formatDuration(GraceHardThreshold-cacheAge),
			),
			CanProceed:   true,
			WriteAllowed: true,
		}
	default:
		return GraceCheckResult{
			State:     GraceHard,
			CacheAge:  cacheAge,
			ExpiresAt: expiresAt,
			Tier:      entry.Tier,
			Message: fmt.Sprintf(
				"License validation expired (%s offline). "+
					"Paid plugins are in read-only mode. Connect to refresh.",
				formatDuration(cacheAge),
			),
			CanProceed:   true,
			WriteAllowed: false,
		}
	}
}

// postExpiryGraceState evaluates the grace state for a license whose
// server-reported expiry has passed. Within PostExpiryGraceWindow (30 days)
// of expiry, paid plugins keep working with a warning (GracePostExpiry).
// Beyond it, the license goes dormant (GraceExpired). Split out of
// DetermineGraceState to keep that function under the repo's 50-line cap.
func postExpiryGraceState(entry *CacheEntry, expiresAt time.Time, cacheAge time.Duration, now time.Time) GraceCheckResult {
	sinceExpiry := now.Sub(expiresAt)
	graceDays := int(PostExpiryGraceWindow.Hours() / 24)

	if sinceExpiry <= PostExpiryGraceWindow {
		remaining := PostExpiryGraceWindow - sinceExpiry
		return GraceCheckResult{
			State:     GracePostExpiry,
			CacheAge:  cacheAge,
			ExpiresAt: expiresAt,
			Tier:      entry.Tier,
			Message: fmt.Sprintf(
				"License expired on %s. Paid plugins continue to work during a %d-day post-expiry grace period; %s remaining before they go dormant. Renew at https://nself.org/pricing.",
				expiresAt.Format("2006-01-02"), graceDays, formatDuration(remaining),
			),
			CanProceed:   true,
			WriteAllowed: true,
		}
	}

	return GraceCheckResult{
		State:     GraceExpired,
		CacheAge:  cacheAge,
		ExpiresAt: expiresAt,
		Tier:      entry.Tier,
		Message: fmt.Sprintf(
			"License expired on %s. The %d-day post-expiry grace period has ended; paid plugins are now dormant. Renew at https://nself.org/pricing",
			expiresAt.Format("2006-01-02"), graceDays,
		),
		CanProceed:   false,
		WriteAllowed: false,
	}
}

// IsWriteAllowed checks if write operations on paid plugins are permitted
// given the current grace state.
func (r GraceCheckResult) IsWriteAllowed() bool {
	return r.WriteAllowed
}

// NeedsBanner returns true if a warning banner should be displayed.
func (r GraceCheckResult) NeedsBanner() bool {
	return r.State == GraceSoft || r.State == GraceHard || r.State == GracePostExpiry
}

// formatDuration returns a human-readable duration string.
func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	hours := int(d.Hours())
	if hours < 48 {
		return fmt.Sprintf("%d hours", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%d days", days)
}
