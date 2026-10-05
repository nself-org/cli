package license

// term_regress_test.go: P7-PLUG-63 recheck 2. A cached licence is bounded by
// its signed term (expiry plus the 30-day post-expiry grace), post-expiry grace
// still obeys the 7-day offline ceiling, and a clock rolled back behind signed
// or previously trusted time makes the cache untrusted. v1.4 is unchanged.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// signedEntry is a genuine ping-signed cache entry for testLicenseKey: the
// licence expires expiresIn from now, the jwt was issued issuedAgo ago (a
// negative value means in the future), fetched_at is restamped to now.
func signedEntry(t *testing.T, expiresIn, issuedAgo time.Duration) *CacheEntry {
	t.Helper()
	iat := time.Now().Add(-issuedAgo)
	stub := newPingStubWith(t,
		func(m map[string]any) { m["expires_at"] = time.Now().Add(expiresIn).UTC().Format(time.RFC3339) },
		func(c map[string]any) { c["iat"], c["exp"] = iat.Unix(), iat.Add(24*time.Hour).Unix() })
	var resp ValidateResponse
	if err := json.Unmarshal(stub.body, &resp); err != nil {
		t.Fatal(err)
	}
	resp.RawBody, resp.BodySig = string(stub.body), hex.EncodeToString(ed25519.Sign(stub.priv, stub.body))
	e := responseToCache(testLicenseKey, &resp)
	e.FetchedAt = time.Now().Unix()
	return e
}

func offlineWith(t *testing.T, e *CacheEntry, failOpen string) {
	t.Helper()
	withTempCachePath(t)
	checkerCacheDir(t)
	seedCache(t, e)
	offline(t)
	t.Setenv("NSELF_LICENSE_FAIL_OPEN", failOpen)
}

func bundleOK(t *testing.T) bool {
	t.Helper()
	var ok bool
	captureStderr(t, func() { ok, _ = BundleEntitled(context.Background(), testLicenseKey, "chat") })
	return ok
}

// MF7: NSELF_LICENSE_FAIL_OPEN is unbounded by cache age but never by the licence term.
func TestFailOpenStopsAtTheSignedTermV15(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		name      string
		expiresIn time.Duration
		issuedAgo time.Duration
		v15       bool
	}{
		{"live licence, cache issued 400 days ago", 30 * day, 400 * day, true},
		{"expired 10 days ago (inside the 30-day grace)", -10 * day, 200 * day, true},
		{"expired 29 days ago", -29 * day, 300 * day, true},
		{"expired 31 days ago", -31 * day, 300 * day, false},
		{"expired 365 days ago, iat 400 days old (reviewer probe)", -365 * day, 400 * day, false},
	}
	for _, c := range cases {
		compattest.Both(t, func(t *testing.T) {
			want := true // v1.4 never looked at the term
			if compatMode15() {
				want = c.v15
			}
			e := signedEntry(t, c.expiresIn, c.issuedAgo)
			offlineWith(t, e, "1")
			if got := bundleOK(t); got != want {
				t.Errorf("%s: fail-open granted=%v, want %v", c.name, got, want)
			}
		})
	}
}

func TestCacheWithinTermEdges(t *testing.T) {
	compattest.Set(t, true)
	exp := time.Unix(1_800_000_000, 0)
	e := &CacheEntry{ExpiresAt: exp.Unix()}
	end := exp.Add(PostExpiryGraceWindow)
	for name, c := range map[string]struct {
		now time.Time
		ok  bool
	}{
		"before expiry":        {exp.Add(-time.Hour), true},
		"exactly at expiry":    {exp, true},
		"exactly at grace end": {end, true},
		"1 s after grace end":  {end.Add(time.Second), false},
	} {
		if got := cacheWithinTerm(e, c.now); got != c.ok {
			t.Errorf("%s: %v, want %v", name, got, c.ok)
		}
	}
	if !cacheWithinTerm(&CacheEntry{}, end.Add(1000*24*time.Hour)) {
		t.Error("an entry with no expiry has no term to enforce")
	}
	compattest.Set(t, false)
	if !cacheWithinTerm(e, end.Add(time.Hour)) {
		t.Error("v1.4 does not enforce the term here")
	}
}

// SF2: after expiry the 30-day grace still needs a cache younger than the 7-day offline ceiling.
func TestPostExpiryGraceObeysTheOfflineCeilingV15(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		name      string
		expiresIn time.Duration
		issuedAgo time.Duration
		v15       bool
	}{
		{"expired 10 d ago, cache issued 2 days ago", -10 * day, 2 * day, true},
		{"expired 10 d ago, cache issued 6 days ago", -10 * day, 6 * day, true},
		{"expired 10 d ago, cache issued 8 days ago", -10 * day, 8 * day, false},
		{"expired 10 d ago, cache issued 200 days ago (reviewer probe)", -10 * day, 200 * day, false},
		{"expired 31 d ago, cache issued 2 days ago", -31 * day, 2 * day, false},
	}
	for _, c := range cases {
		compattest.Both(t, func(t *testing.T) {
			e := signedEntry(t, c.expiresIn, c.issuedAgo)
			offlineWith(t, e, "")
			want := c.expiresIn > -30*day // v1.4: only the 30-day window counts (fetched_at is fresh)
			if compatMode15() {
				want = c.v15
			}
			if got := bundleOK(t); got != want {
				t.Errorf("%s: BundleEntitled granted=%v, want %v", c.name, got, want)
			}
			if res, _ := ValidateFull(context.Background(), testLicenseKey); res.WriteAllowed != want {
				t.Errorf("%s: ValidateFull write allowed=%v, want %v (%s)", c.name, res.WriteAllowed, want, res.Message)
			}
		})
	}
}

func TestGraceStateForPinsTheCeilingToPostExpiryOnly(t *testing.T) {
	compattest.Set(t, true)
	day := 24 * time.Hour
	old := signedEntry(t, -10*day, 8*day)
	if g := graceStateFor(old); g.State != GracePostExpiry || g.CanProceed != true || g.WriteAllowed {
		t.Errorf("old post-expiry cache: %+v, want read-only post-expiry", g)
	}
	young := signedEntry(t, -10*day, day)
	if g := graceStateFor(young); !g.WriteAllowed || g.State != GracePostExpiry {
		t.Errorf("young post-expiry cache: %+v, want write allowed", g)
	}
	live := signedEntry(t, 30*day, time.Hour)
	if g := graceStateFor(live); !g.WriteAllowed || g.State != GraceValid {
		t.Errorf("live cache: %+v", g)
	}
}

// SF3: a clock behind signed or previously trusted time makes the cache untrusted in v1.5.
func TestClockRollbackMakesTheCacheUntrustedV15(t *testing.T) {
	day := 24 * time.Hour
	for _, failOpen := range []string{"", "1"} {
		compattest.Both(t, func(t *testing.T) {
			want := true // v1.4 has no clock check
			// reviewer probe: signed iat 30 days in the future
			e := signedEntry(t, 60*day, -30*day)
			offlineWith(t, e, failOpen)
			if compatMode15() {
				want = false
				if e.clockTrusted(time.Now()) {
					t.Error("a signed iat 30 days ahead of the clock must not be trusted")
				}
			}
			if got := bundleOK(t); got != want {
				t.Errorf("fail-open=%q: granted=%v, want %v", failOpen, got, want)
			}
			if res, _ := ValidateFull(context.Background(), testLicenseKey); res.Valid != want {
				t.Errorf("fail-open=%q: ValidateFull valid=%v, want %v", failOpen, res.Valid, want)
			}
		})
	}
}

func TestTrustedTimeMarkDetectsRollbackAndRecovers(t *testing.T) {
	compattest.Set(t, true)
	e := signedEntry(t, 30*24*time.Hour, time.Hour)
	withTempCachePath(t)
	checkerCacheDir(t)
	seedCache(t, e)
	offline(t)
	mark, ok := clockMarkPath()
	if !ok {
		t.Fatal("no clock mark path")
	}
	now := time.Now()

	if !bundleOK(t) {
		t.Fatal("precondition: a genuine, fresh cache grants offline")
	}
	if got := readTrustedTime(); got.Before(now.Add(-time.Minute)) {
		t.Errorf("a trusted decision must raise the mark to now, got %v", got)
	}
	// Earlier checks saw a time 2 h ahead of this clock: the clock was rolled back.
	writeTrustedTime(now.Add(2 * time.Hour))
	if e.clockTrusted(now) || bundleOK(t) {
		t.Error("a clock 2 h behind the highest trusted time must make the cache untrusted")
	}
	// Inside the tolerance it is fine, and the mark is never lowered by a check.
	writeTrustedTime(now.Add(10*time.Minute - time.Minute))
	if !e.clockTrusted(now) {
		t.Error("a clock within the tolerance must still be trusted")
	}
	writeTrustedTime(now.Add(10*time.Minute + time.Minute))
	if e.clockTrusted(now) {
		t.Error("a clock just past the tolerance must not be trusted")
	}
	before := readTrustedTime()
	noteTrustedTime(now)
	if got := readTrustedTime(); !got.Equal(before) {
		t.Errorf("noteTrustedTime lowered the mark: %v -> %v", before, got)
	}
	// Going online with a verified reply resets the mark to real time.
	newPingStub(t, nil)
	t.Setenv("LICENSE_CACHE_PATH", mark[:len(mark)-len("license.clock")]+"license.json")
	if res, err := RefreshCache(context.Background(), testLicenseKey); err != nil || !res.Valid {
		t.Fatalf("RefreshCache: %+v, %v", res, err)
	}
	if b, _ := os.ReadFile(mark); len(b) == 0 {
		t.Fatal("the mark must exist")
	} else if n, _ := strconv.ParseInt(string(b), 10, 64); n > time.Now().Add(time.Minute).Unix() {
		t.Errorf("the online reply must reset the mark to its iat, got %d", n)
	}
	offline(t)
	if !bundleOK(t) {
		t.Error("after going online the cache is trusted again")
	}
	// A garbage or missing mark file means no evidence, not a lockout.
	_ = os.WriteFile(mark, []byte("not a number"), 0o600)
	if !readTrustedTime().IsZero() || !e.clockTrusted(now) {
		t.Error("an unreadable mark must read as no evidence")
	}
	for _, v := range []string{"0", "-5", ""} {
		_ = os.WriteFile(mark, []byte(v), 0o600)
		if !readTrustedTime().IsZero() {
			t.Errorf("mark %q must read as no evidence", v)
		}
	}
	_ = os.Remove(mark)
	if !readTrustedTime().IsZero() {
		t.Error("a missing mark must read as zero")
	}
}

// Validate (the fail-open validator) refuses a cache the clock cannot be trusted against.
func TestValidatorRefusesAfterClockRollbackV15(t *testing.T) {
	compattest.Set(t, true)
	e := signedEntry(t, 60*24*time.Hour, -30*24*time.Hour)
	offlineWith(t, e, "")
	warn := func(string) {}
	vr, _ := Validate(context.Background(), testLicenseKey, &ValidatorOptions{PingURL: PingURL(), WarnOnce: warn})
	if vr.CanProceed {
		t.Errorf("Validate granted on a rolled-back clock: %+v", vr)
	}
}

// The signed-iat check at its exact edge: an iat the skew allowance away is fine, one second further is a rollback.
func TestSignedIssuedAtSkewEdgeV15(t *testing.T) {
	compattest.Set(t, true)
	withTempCachePath(t)
	e := signedEntry(t, 30*24*time.Hour, time.Hour)
	iat, ok := e.signedIssuedAt()
	if !ok {
		t.Fatal("entry has no signed iat")
	}
	if !e.clockTrusted(time.Unix(iat-replySkewSec, 0)) {
		t.Error("a clock exactly at iat minus the skew allowance is still trusted")
	}
	if e.clockTrusted(time.Unix(iat-replySkewSec-1, 0)) {
		t.Error("a clock 1 s further behind must not be trusted")
	}
}
