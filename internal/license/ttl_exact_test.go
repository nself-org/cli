package license

import (
	"testing"
	"time"
)

// TestLicenseConstantsPinned pins the value of every timing constant that mutation tooling cannot reach (D-0226).
func TestLicenseConstantsPinned(t *testing.T) {
	cases := []struct {
		name      string
		got, want time.Duration
	}{
		{"TTLFree", TTLFree, 0},
		{"TTLPro", TTLPro, 168 * time.Hour},
		{"TTLPlus", TTLPlus, 720 * time.Hour},
		{"PreExpiryWarnStart", PreExpiryWarnStart, 48 * time.Hour},
		{"PreExpiryWarnEnd", PreExpiryWarnEnd, 0},
		{"PostExpiryGraceWindow", PostExpiryGraceWindow, 720 * time.Hour},
		{"GraceSoftThreshold", GraceSoftThreshold, 72 * time.Hour},
		{"GraceHardThreshold", GraceHardThreshold, 168 * time.Hour},
		{"RevocationFailOpenStaleness", RevocationFailOpenStaleness, 168 * time.Hour},
		{"RevocationRefreshInterval", RevocationRefreshInterval, time.Hour},
		{"revocationHTTPTimeout", revocationHTTPTimeout, 10 * time.Second},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if revocationCacheFile != "revocation-cache.json" {
		t.Errorf("revocationCacheFile = %q", revocationCacheFile)
	}
	if replySkewSec != 300 || replyMaxWindowSec != 86400 {
		t.Errorf("replySkewSec = %d, replyMaxWindowSec = %d, want 300 and 86400", replySkewSec, replyMaxWindowSec)
	}
}

// TestCacheTTLForTierExact pins the tier to TTL mapping, case-insensitively (D-0226).
func TestCacheTTLForTierExact(t *testing.T) {
	cases := map[string]time.Duration{
		"": 0, "free": 0, "FREE": 0, "Free": 0,
		"owner": TTLPlus, "OWNER": TTLPlus, "nself+": TTLPlus, "plus": TTLPlus, "Plus": TTLPlus,
		"max": TTLPlus, "Max": TTLPlus, "enterprise": TTLPlus, "ENTERPRISE": TTLPlus,
		"business": TTLPlus, "Business": TTLPlus,
		"pro": TTLPro, "Pro": TTLPro, "bundle-x": TTLPro, "freebie": TTLPro, "unknown": TTLPro,
	}
	for tier, want := range cases {
		if got := CacheTTLForTier(tier); got != want {
			t.Errorf("CacheTTLForTier(%q) = %v, want %v", tier, got, want)
		}
	}
}

// TestCacheTTLExpiryExact pins that free expires at fetch time and paid tiers add their exact TTL (D-0226).
func TestCacheTTLExpiryExact(t *testing.T) {
	at := time.Unix(1_800_000_000, 0)
	cases := []struct {
		tier string
		want time.Time
	}{
		{"free", at},
		{"pro", at.Add(168 * time.Hour)},
		{"plus", at.Add(720 * time.Hour)},
	}
	for _, c := range cases {
		if got := CacheTTLExpiry(c.tier, at); !got.Equal(c.want) {
			t.Errorf("CacheTTLExpiry(%q) = %v, want %v", c.tier, got, c.want)
		}
	}
}

// TestPreExpiryWarningAtExact pins the warning window edges and the exact day counts (D-0226).
func TestPreExpiryWarningAtExact(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	// entryLeft is a pro entry whose cache expires `left` from now.
	entryLeft := func(left time.Duration) *CacheEntry {
		return &CacheEntry{Tier: "pro", FetchedAt: now.Add(left - TTLPro).Unix()}
	}
	const post = "Cache expired. Run nself license revalidate after connecting."
	cases := []struct {
		name string
		left time.Duration
		msg  string
		post bool
	}{
		{"3 days left", 3 * exactDay, "", false},
		{"window start + 1s", PreExpiryWarnStart + time.Second, "", false},
		{"window start", PreExpiryWarnStart, "License cache expires in 3 days. Connect to revalidate.", false},
		{"36h left", 36 * time.Hour, "License cache expires in 2 days. Connect to revalidate.", false},
		{"24h left", exactDay, "License cache expires in 2 days. Connect to revalidate.", false},
		{"24h-1s left", exactDay - time.Second, "License cache expires in 1 day. Connect to revalidate.", false},
		{"12h left", 12 * time.Hour, "License cache expires in 1 day. Connect to revalidate.", false},
		{"1s left", time.Second, "License cache expires in 1 day. Connect to revalidate.", false},
		{"expiring now", 0, post, true},
		{"1s past expiry", -time.Second, post, true},
		{"5 days past expiry", -5 * exactDay, post, true},
	}
	for _, c := range cases {
		msg, isPost := preExpiryWarningAt(entryLeft(c.left), now)
		if msg != c.msg || isPost != c.post {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, msg, isPost, c.msg, c.post)
		}
	}
	if msg, isPost := preExpiryWarningAt(nil, now); msg != "" || isPost {
		t.Errorf("nil entry: got (%q, %v)", msg, isPost)
	}
	free := &CacheEntry{Tier: "free", FetchedAt: now.Add(-100 * exactDay).Unix()}
	if msg, isPost := preExpiryWarningAt(free, now); msg != "" || isPost {
		t.Errorf("free tier: got (%q, %v)", msg, isPost)
	}
	big := &CacheEntry{Tier: "plus", FetchedAt: now.Add(-TTLPlus + 20*time.Hour).Unix()}
	if msg, _ := preExpiryWarningAt(big, now); msg != "License cache expires in 1 day. Connect to revalidate." {
		t.Errorf("plus tier 20h left: got %q", msg)
	}
}
