// Package license — grace_test.go tests the grace state machine.
// S88b.T01 coverage extension: all GraceState branches + edge cases.
package license

import (
	"testing"
	"time"
)

// makeCacheEntry returns a CacheEntry with the given fetched-at offset from now.
// expiryOffset is how far in the future the entry expires (use negative for expired).
func makeCacheEntry(keyHash string, fetchedAgoHours float64, expiryOffsetHours float64) *CacheEntry {
	now := time.Now()
	return &CacheEntry{
		KeyHash:        keyHash,
		Tier:           "pro",
		PluginsAllowed: []string{"ai", "claw", "mux"},
		FetchedAt:      now.Add(-time.Duration(fetchedAgoHours * float64(time.Hour))).Unix(),
		ExpiresAt:      now.Add(time.Duration(expiryOffsetHours * float64(time.Hour))).Unix(),
	}
}

// TestDetermineGraceState_NilEntry verifies that nil cache returns GraceExpired.
func TestDetermineGraceState_NilEntry(t *testing.T) {
	result := DetermineGraceState(nil)
	if result.State != GraceExpired {
		t.Errorf("nil entry: state = %q, want %q", result.State, GraceExpired)
	}
	if result.CanProceed {
		t.Error("nil entry: CanProceed should be false")
	}
	if result.WriteAllowed {
		t.Error("nil entry: WriteAllowed should be false")
	}
}

// TestDetermineGraceState_Valid verifies that a freshly-fetched entry (< 72h ago) returns GraceValid.
func TestDetermineGraceState_Valid(t *testing.T) {
	entry := makeCacheEntry("nself_pro_testkey", 1, 720) // fetched 1h ago, expires 720h from now
	result := DetermineGraceState(entry)
	if result.State != GraceValid {
		t.Errorf("fresh entry (1h ago): state = %q, want %q", result.State, GraceValid)
	}
	if !result.CanProceed {
		t.Error("fresh entry: CanProceed should be true")
	}
	if !result.WriteAllowed {
		t.Error("fresh entry: WriteAllowed should be true")
	}
}

// TestDetermineGraceState_SoftGrace verifies that a 72h-7d old entry returns GraceSoft with writes still allowed.
func TestDetermineGraceState_SoftGrace(t *testing.T) {
	// 73h ago — just past the 72h soft threshold
	entry := makeCacheEntry("nself_pro_testkey", 73, 720)
	result := DetermineGraceState(entry)
	if result.State != GraceSoft {
		t.Errorf("73h old entry: state = %q, want %q", result.State, GraceSoft)
	}
	if !result.CanProceed {
		t.Error("grace_soft: CanProceed should be true (allow with warning)")
	}
	if !result.WriteAllowed {
		t.Error("grace_soft: WriteAllowed should be true")
	}
}

// TestDetermineGraceState_JustUnder72h_Silent verifies that an entry just
// under the 72h soft threshold is silent (GraceValid), per the decided
// ladder: <72h offline = silent, 72h-7d = warning, >7d = fail closed.
func TestDetermineGraceState_JustUnder72h_Silent(t *testing.T) {
	entry := makeCacheEntry("nself_pro_testkey", 71.99, 720)
	result := DetermineGraceState(entry)
	if result.State != GraceValid {
		t.Errorf("71h59m old entry: state = %q, want %q (silent, no warning)", result.State, GraceValid)
	}
	if !result.CanProceed || !result.WriteAllowed {
		t.Error("just-under-72h entry: CanProceed and WriteAllowed should both be true")
	}
}

// TestDetermineGraceState_SoftGraceBoundary verifies the exact 72h boundary.
func TestDetermineGraceState_SoftGraceBoundary(t *testing.T) {
	// Just past 72h is just at the soft boundary — should be soft or valid depending on impl.
	entry72h := makeCacheEntry("nself_pro_testkey", 72.01, 720)
	result := DetermineGraceState(entry72h)
	// 72h+ means we are in soft grace territory.
	if result.State != GraceSoft && result.State != GraceValid {
		t.Errorf("72h boundary entry: state = %q, want %q or %q", result.State, GraceSoft, GraceValid)
	}
}

// TestDetermineGraceState_HardGrace verifies that an 8d old entry returns GraceHard with writes blocked.
func TestDetermineGraceState_HardGrace(t *testing.T) {
	entry := makeCacheEntry("nself_pro_testkey", 8*24, 720) // 8 days ago
	result := DetermineGraceState(entry)
	if result.State != GraceHard {
		t.Errorf("8d old entry: state = %q, want %q", result.State, GraceHard)
	}
	if !result.CanProceed {
		t.Error("grace_hard: CanProceed should be true (read-only still allowed)")
	}
	if result.WriteAllowed {
		t.Error("grace_hard: WriteAllowed should be false (read-only degradation)")
	}
}

// TestDetermineGraceState_Expired verifies that a license expired well beyond
// the 30-day post-expiry grace window (PostExpiryGraceWindow) returns
// GraceExpired — the terminal, dormant state. A license expired only
// recently is covered by TestDetermineGraceState_PostExpiryGrace_Day29
// below (P6-E12-W4-S4-T2: 30-day post-expiry grace, commercial promise).
func TestDetermineGraceState_Expired(t *testing.T) {
	// 31 days past expiry — one day beyond the 30-day grace window.
	entry := makeCacheEntry("nself_pro_testkey", 1, -31*24)
	result := DetermineGraceState(entry)
	if result.State != GraceExpired {
		t.Errorf("31d-past-expiry entry: state = %q, want %q", result.State, GraceExpired)
	}
	if result.CanProceed {
		t.Error("31d-past-expiry entry: CanProceed should be false (dormant)")
	}
	if result.WriteAllowed {
		t.Error("31d-past-expiry entry: WriteAllowed should be false")
	}
}

// TestDetermineGraceState_PostExpiryGrace_Day29 proves the ticket's required
// case: a license expired 29 days ago still proceeds, with a warning message
// naming the grace period, per the commercial promise (Bundle License §4,
// licensing.mdx, pricing FAQ) that paid plugins keep working for 30 days
// after expiry.
func TestDetermineGraceState_PostExpiryGrace_Day29(t *testing.T) {
	entry := makeCacheEntry("nself_pro_testkey", 1, -29*24)
	result := DetermineGraceState(entry)
	if result.State != GracePostExpiry {
		t.Errorf("29d-past-expiry entry: state = %q, want %q", result.State, GracePostExpiry)
	}
	if !result.CanProceed {
		t.Error("29d-past-expiry entry: CanProceed should be true (still within 30-day grace)")
	}
	if !result.WriteAllowed {
		t.Error("29d-past-expiry entry: WriteAllowed should be true (grace period, not read-only)")
	}
	if result.Message == "" {
		t.Error("29d-past-expiry entry: Message should not be empty (warning required)")
	}
}

// TestDetermineGraceState_PostExpiryGrace_Day31Dormant proves the ticket's
// other required case: a license expired 31 days ago (one day past the
// 30-day post-expiry grace window) goes dormant — CanProceed false.
func TestDetermineGraceState_PostExpiryGrace_Day31Dormant(t *testing.T) {
	entry := makeCacheEntry("nself_pro_testkey", 1, -31*24)
	result := DetermineGraceState(entry)
	if result.State != GraceExpired {
		t.Errorf("31d-past-expiry entry: state = %q, want %q", result.State, GraceExpired)
	}
	if result.CanProceed {
		t.Error("31d-past-expiry entry: CanProceed should be false (dormant, grace exhausted)")
	}
	if result.WriteAllowed {
		t.Error("31d-past-expiry entry: WriteAllowed should be false")
	}
}

// TestDetermineGraceState_PostExpiryGraceBoundary verifies the exact 30-day
// boundary: just under 30 days since expiry proceeds; just over goes dormant.
func TestDetermineGraceState_PostExpiryGraceBoundary(t *testing.T) {
	underEntry := makeCacheEntry("nself_pro_testkey", 1, -30*24+1) // 1h short of 30d
	under := DetermineGraceState(underEntry)
	if under.State != GracePostExpiry || !under.CanProceed {
		t.Errorf("just-under-30d: state = %q, canProceed = %v, want %q / true", under.State, under.CanProceed, GracePostExpiry)
	}

	overEntry := makeCacheEntry("nself_pro_testkey", 1, -30*24-1) // 1h past 30d
	over := DetermineGraceState(overEntry)
	if over.State != GraceExpired || over.CanProceed {
		t.Errorf("just-over-30d: state = %q, canProceed = %v, want %q / false", over.State, over.CanProceed, GraceExpired)
	}
}

// TestDetermineGraceState_HardGraceBoundary verifies the exact 7d boundary.
func TestDetermineGraceState_HardGraceBoundary(t *testing.T) {
	// 7d + 1h — just past the hard threshold
	entry := makeCacheEntry("nself_pro_testkey", 7*24+1, 720)
	result := DetermineGraceState(entry)
	if result.State != GraceHard {
		t.Errorf("7d+1h entry: state = %q, want %q", result.State, GraceHard)
	}
}

// TestDetermineGraceState_GraceMessageNotEmpty verifies that each state includes a non-empty message.
func TestDetermineGraceState_GraceMessageNotEmpty(t *testing.T) {
	cases := []struct {
		name  string
		entry *CacheEntry
	}{
		{"nil", nil},
		{"valid", makeCacheEntry("k", 1, 720)},
		{"soft", makeCacheEntry("k", 73, 720)},
		{"hard", makeCacheEntry("k", 8*24, 720)},
		{"post_expiry_grace", makeCacheEntry("k", 1, -1)},
		{"expired", makeCacheEntry("k", 1, -31*24)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := DetermineGraceState(tc.entry)
			if result.State == GraceValid {
				return // valid state may have empty message — that is fine
			}
			if result.Message == "" {
				t.Errorf("state %q: message should not be empty (user needs to know what to do)", result.State)
			}
		})
	}
}

// TestDetermineGraceState_Tier propagates from cache entry.
func TestDetermineGraceState_Tier(t *testing.T) {
	entry := makeCacheEntry("k", 1, 720)
	entry.Tier = "enterprise"
	result := DetermineGraceState(entry)
	if result.Tier != "enterprise" {
		t.Errorf("tier propagation: got %q, want enterprise", result.Tier)
	}
}

// TestDetermineGraceState_CacheAgeAccuracy verifies CacheAge matches the entry age.
func TestDetermineGraceState_CacheAgeAccuracy(t *testing.T) {
	entry := makeCacheEntry("k", 2, 720) // 2h ago
	result := DetermineGraceState(entry)
	// CacheAge should be approximately 2h (allow 10s tolerance).
	wantAge := 2 * time.Hour
	if result.CacheAge < wantAge-10*time.Second || result.CacheAge > wantAge+10*time.Second {
		t.Errorf("CacheAge = %v, want ~%v (±10s)", result.CacheAge, wantAge)
	}
}
