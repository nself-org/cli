package license

import (
	"testing"
	"time"
)

const exactDay = 24 * time.Hour

// TestBannerSoftExactDays pins the GraceSoft banner text and its remaining-days arithmetic (D-0226).
func TestBannerSoftExactDays(t *testing.T) {
	cases := []struct {
		name string
		age  time.Duration
		want string
	}{
		// remaining 36h -> 1 full day; the "< 1" edge must not fire at 1.
		{"5.5 days old", 5*exactDay + 12*time.Hour, "License server unreachable for 5 days. Plugins will stop in 1 days if not restored."},
		// remaining 12h -> 0 full days -> the within-hours text.
		{"6.5 days old", 6*exactDay + 12*time.Hour, "License server unreachable. Plugins will stop within hours if not restored."},
		// remaining exactly 24h -> 1 day, not hours.
		{"6 days old", 6 * exactDay, "License server unreachable for 6 days. Plugins will stop in 1 days if not restored."},
		// remaining 5d -> 5 days.
		{"2 days old", 2 * exactDay, "License server unreachable for 2 days. Plugins will stop in 5 days if not restored."},
		// remaining 2.5d -> 2 days (hours/24 truncates).
		{"4.5 days old", 4*exactDay + 12*time.Hour, "License server unreachable for 4 days. Plugins will stop in 2 days if not restored."},
	}
	for _, c := range cases {
		got := BannerAtWarning(GraceCheckResult{State: GraceSoft, CacheAge: c.age})
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestBannerPostExpiryExactDays pins the GracePostExpiry banner text against the wall clock, with half-day margins (D-0226).
func TestBannerPostExpiryExactDays(t *testing.T) {
	cases := []struct {
		name  string
		since time.Duration
		want  string
	}{
		// remaining 1.5d -> 1 full day.
		{"28.5 days after expiry", 28*exactDay + 12*time.Hour, "License expired. Paid plugins go dormant in 1 days. Renew at https://nself.org/pricing"},
		// remaining 0.5d -> within a day.
		{"29.5 days after expiry", 29*exactDay + 12*time.Hour, "License expired. Paid plugins go dormant within a day. Renew at https://nself.org/pricing"},
		// remaining 9.5d -> 9 days.
		{"20.5 days after expiry", 20*exactDay + 12*time.Hour, "License expired. Paid plugins go dormant in 9 days. Renew at https://nself.org/pricing"},
		// remaining 29.5d -> 29 days.
		{"just expired", 12 * time.Hour, "License expired. Paid plugins go dormant in 29 days. Renew at https://nself.org/pricing"},
	}
	for _, c := range cases {
		got := BannerAtWarning(GraceCheckResult{State: GracePostExpiry, ExpiresAt: time.Now().Add(-c.since)})
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestBannerFixedTexts pins the exact text of the states that carry no arithmetic (D-0226).
func TestBannerFixedTexts(t *testing.T) {
	cases := []struct {
		state GraceState
		want  string
	}{
		{GraceHard, "License grace period expired. Paid plugin writes are disabled. Connect to restore."},
		{GraceExpired, "License expired. Paid plugins are disabled. Renew at https://nself.org/pricing"},
		{GraceRevoked, "License revoked. Contact support at https://nself.org/support"},
		{GraceValid, ""},
		{GraceState("bogus"), ""},
	}
	for _, c := range cases {
		if got := BannerAtWarning(GraceCheckResult{State: c.state}); got != c.want {
			t.Errorf("state %s: got %q, want %q", c.state, got, c.want)
		}
	}
}

// TestBannerAtExpiryExact pins the hard-stop banner text (D-0226).
func TestBannerAtExpiryExact(t *testing.T) {
	want := "License grace period expired. New paid plugin operations return 402. " +
		"GET endpoints serve cached data. Free plugins are unaffected. " +
		"Restore connectivity and run 'nself license refresh' to resume."
	if got := BannerAtExpiry(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
