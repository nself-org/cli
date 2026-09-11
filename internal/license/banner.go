// Package license — banner.go provides warning banner text for grace period
// states. Used by CLI header output and nself-admin UI.
package license

import (
	"fmt"
	"time"
)

// BannerAtWarning returns the yellow-banner text shown when the license
// server has been unreachable for 7+ days (grace_soft state). Returns empty
// string if no banner is needed.
func BannerAtWarning(result GraceCheckResult) string {
	switch result.State {
	case GraceSoft:
		remaining := GraceHardThreshold - result.CacheAge
		days := int(remaining.Hours()) / 24
		if days < 1 {
			return "License server unreachable. Plugins will stop within hours if not restored."
		}
		return fmt.Sprintf(
			"License server unreachable for %s. Plugins will stop in %d days if not restored.",
			formatDuration(result.CacheAge), days,
		)
	case GraceHard:
		return "License grace period expired. Paid plugin writes are disabled. Connect to restore."
	case GracePostExpiry:
		// Remaining is relative to time since the license's own expiry
		// (ExpiresAt), not CacheAge (time since last successful fetch) —
		// those are different clocks.
		sinceExpiry := time.Since(result.ExpiresAt)
		remaining := PostExpiryGraceWindow - sinceExpiry
		days := int(remaining.Hours() / 24)
		if days < 1 {
			return "License expired. Paid plugins go dormant within a day. Renew at https://nself.org/pricing"
		}
		return fmt.Sprintf(
			"License expired. Paid plugins go dormant in %d days. Renew at https://nself.org/pricing",
			days,
		)
	case GraceExpired:
		return "License expired. Paid plugins are disabled. Renew at https://nself.org/pricing"
	case GraceRevoked:
		return "License revoked. Contact support at https://nself.org/support"
	default:
		return ""
	}
}

// BannerAtExpiry returns the hard-stop banner text when grace period is
// exhausted (day 14+).
func BannerAtExpiry() string {
	return "License grace period expired. New paid plugin operations return 402. " +
		"GET endpoints serve cached data. Free plugins are unaffected. " +
		"Restore connectivity and run 'nself license refresh' to resume."
}
