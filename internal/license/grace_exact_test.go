package license

import (
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// graceNow is the fixed instant the grace tests evaluate at (whole seconds).
var graceNow = time.Unix(1_800_000_000, 0)

// freshAt returns an unsigned entry fetched age before graceNow whose licence expires in 100 days.
func freshAt(age time.Duration) *CacheEntry {
	return &CacheEntry{
		KeyHash: "h", Tier: "pro",
		FetchedAt: graceNow.Add(-age).Unix(),
		ExpiresAt: graceNow.Add(100 * exactDay).Unix(),
	}
}

// TestGraceZeroExpiryIsNeverExpired pins that expires_at 0 (no expiry) with a fresh cache is GraceValid (D-0226).
func TestGraceZeroExpiryIsNeverExpired(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		g := graceStateAt(&CacheEntry{Tier: "pro", FetchedAt: graceNow.Unix(), ExpiresAt: 0}, graceNow)
		if g.State != GraceValid || !g.CanProceed || !g.WriteAllowed || g.Message != "" {
			t.Errorf("got %+v, want a silent GraceValid", g)
		}
	})
}

// TestGraceExpiryBoundary pins that now == expires_at is still valid and one second later is post-expiry (D-0226).
func TestGraceExpiryBoundary(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		e := freshAt(0)
		e.ExpiresAt = graceNow.Unix()
		if g := graceStateAt(e, graceNow); g.State != GraceValid {
			t.Errorf("at expiry: state %s, want valid", g.State)
		}
		if g := graceStateAt(e, graceNow.Add(time.Second)); g.State != GracePostExpiry {
			t.Errorf("1s after expiry: state %s, want grace_post_expiry", g.State)
		}
	})
}

// TestGraceNilEntryExact pins the no-cache result (D-0226).
func TestGraceNilEntryExact(t *testing.T) {
	g := graceStateAt(nil, graceNow)
	if g.State != GraceExpired || g.CanProceed || g.WriteAllowed ||
		g.Message != "No license cache found. Run 'nself license validate' to validate your license." {
		t.Errorf("got %+v", g)
	}
}

// TestGraceFreshnessThresholdsExact pins the 72h and 7d edges and the exact soft/hard texts (D-0226).
func TestGraceFreshnessThresholdsExact(t *testing.T) {
	soft := func(age, left string) string {
		return "License validation is " + age + " old. Connect to the internet to refresh.\nOffline grace period expires in " + left + "."
	}
	cases := []struct {
		name      string
		age       time.Duration
		state     GraceState
		msg       string
		writeable bool
	}{
		{"72h-1s", GraceSoftThreshold - time.Second, GraceValid, "", true},
		{"72h", GraceSoftThreshold, GraceSoft, soft("3 days", "4 days"), true},
		{"4.5 days", 4*exactDay + 12*time.Hour, GraceSoft, soft("4 days", "2 days"), true},
		{"7d-1s", GraceHardThreshold - time.Second, GraceSoft, soft("6 days", "0 minutes"), true},
		{"7d", GraceHardThreshold, GraceHard, "License validation expired (7 days offline). Paid plugins are in read-only mode. Connect to refresh.", false},
		{"10d", 10 * exactDay, GraceHard, "License validation expired (10 days offline). Paid plugins are in read-only mode. Connect to refresh.", false},
	}
	compattest.Both(t, func(t *testing.T) {
		for _, c := range cases {
			g := graceStateAt(freshAt(c.age), graceNow)
			if g.State != c.state || g.Message != c.msg || g.WriteAllowed != c.writeable || !g.CanProceed || g.CacheAge != c.age || g.Tier != "pro" {
				t.Errorf("%s: got state=%s msg=%q write=%v proceed=%v age=%v, want state=%s msg=%q write=%v age=%v",
					c.name, g.State, g.Message, g.WriteAllowed, g.CanProceed, g.CacheAge, c.state, c.msg, c.writeable, c.age)
			}
		}
	})
}

// TestGracePostExpiryExact pins the 30-day post-expiry edge, remaining time and exact texts (D-0226).
func TestGracePostExpiryExact(t *testing.T) {
	expiredAgo := func(since time.Duration) *CacheEntry {
		e := freshAt(time.Hour)
		e.ExpiresAt = graceNow.Add(-since).Unix()
		return e
	}
	date := func(since time.Duration) string {
		return time.Unix(graceNow.Add(-since).Unix(), 0).Format("2006-01-02")
	}
	const tail = " Renew at https://nself.org/pricing."
	in := func(since time.Duration, left string) string {
		return "License expired on " + date(since) + ". Paid plugins continue to work during a 30-day post-expiry grace period; " + left + " remaining before they go dormant." + tail
	}
	dormant := func(since time.Duration) string {
		return "License expired on " + date(since) + ". The 30-day post-expiry grace period has ended; paid plugins are now dormant. Renew at https://nself.org/pricing"
	}
	cases := []struct {
		name  string
		since time.Duration
		state GraceState
		msg   string
		ok    bool
	}{
		{"10 days after", 10 * exactDay, GracePostExpiry, in(10*exactDay, "20 days"), true},
		{"1 day after", exactDay, GracePostExpiry, in(exactDay, "29 days"), true},
		{"exactly 30 days", PostExpiryGraceWindow, GracePostExpiry, in(PostExpiryGraceWindow, "0 minutes"), true},
		{"30 days + 1s", PostExpiryGraceWindow + time.Second, GraceExpired, dormant(PostExpiryGraceWindow + time.Second), false},
		{"45 days after", 45 * exactDay, GraceExpired, dormant(45 * exactDay), false},
	}
	compattest.Set(t, false)
	for _, c := range cases {
		g := graceStateAt(expiredAgo(c.since), graceNow)
		if g.State != c.state || g.Message != c.msg || g.CanProceed != c.ok || g.WriteAllowed != c.ok || g.CacheAge != time.Hour || g.Tier != "pro" {
			t.Errorf("%s: got state=%s msg=%q proceed=%v write=%v age=%v\nwant state=%s msg=%q ok=%v",
				c.name, g.State, g.Message, g.CanProceed, g.WriteAllowed, g.CacheAge, c.state, c.msg, c.ok)
		}
		if !g.ExpiresAt.Equal(graceNow.Add(-c.since)) {
			t.Errorf("%s: ExpiresAt %v, want %v", c.name, g.ExpiresAt, graceNow.Add(-c.since))
		}
	}
}

// TestFormatDurationExact pins the minute/hour/day edges of formatDuration (D-0226).
func TestFormatDurationExact(t *testing.T) {
	cases := map[time.Duration]string{
		0:                               "0 minutes",
		time.Minute:                     "1 minutes",
		59*time.Minute + 59*time.Second: "59 minutes",
		time.Hour:                       "1 hours",
		47 * time.Hour:                  "47 hours",
		47*time.Hour + 59*time.Minute:   "47 hours",
		48 * time.Hour:                  "2 days",
		72 * time.Hour:                  "3 days",
		7*exactDay + 5*time.Hour:        "7 days",
	}
	for d, want := range cases {
		if got := formatDuration(d); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

// TestGraceStateForOfflineCeilingV15 pins the 7-day offline ceiling after expiry: read-only at exactly 7d, writable 1s earlier (D-0226).
func TestGraceStateForOfflineCeilingV15(t *testing.T) {
	const suffix = " The cached validation is older than the 7-day offline ceiling: read-only until you connect."
	postExpiry := func(age time.Duration) *CacheEntry {
		e := freshAt(age)
		e.ExpiresAt = graceNow.Add(-exactDay).Unix()
		return e
	}
	compattest.Set(t, true)
	base := graceStateAt(postExpiry(GraceHardThreshold), graceNow)
	cases := []struct {
		name  string
		entry *CacheEntry
		state GraceState
		write bool
		msg   string
	}{
		{"post-expiry, 7d-1s old", postExpiry(GraceHardThreshold - time.Second), GracePostExpiry, true, graceStateAt(postExpiry(GraceHardThreshold-time.Second), graceNow).Message},
		{"post-expiry, exactly 7d old", postExpiry(GraceHardThreshold), GracePostExpiry, false, base.Message + suffix},
		{"post-expiry, 9d old", postExpiry(9 * exactDay), GracePostExpiry, false, graceStateAt(postExpiry(9*exactDay), graceNow).Message + suffix},
		// Not post-expiry: the hard state keeps its own text and never gains the suffix.
		{"unexpired, 8d old", freshAt(8 * exactDay), GraceHard, false, "License validation expired (8 days offline). Paid plugins are in read-only mode. Connect to refresh."},
	}
	for _, c := range cases {
		g := graceStateFor(c.entry, graceNow)
		if g.State != c.state || g.WriteAllowed != c.write || g.Message != c.msg {
			t.Errorf("%s: got state=%s write=%v msg=%q, want state=%s write=%v msg=%q", c.name, g.State, g.WriteAllowed, g.Message, c.state, c.write, c.msg)
		}
	}
	// v1.4 never applies the extra ceiling.
	compattest.Set(t, false)
	if g := graceStateFor(postExpiry(9*exactDay), graceNow); g.State != GracePostExpiry || !g.WriteAllowed || g.Message != graceStateAt(postExpiry(9*exactDay), graceNow).Message {
		t.Errorf("v1.4: got %+v, want unchanged post-expiry grace", g)
	}
}
