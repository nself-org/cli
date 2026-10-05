package license

// bind_regress_test.go: P7-PLUG-63 recheck holes. In v1.5 a reply is accepted
// only if its SIGNED bytes name the requesting licence (and the requested
// bundle) and carry a validity window; cache age is anchored on the signed
// issued-at, not the unsigned fetched_at. v1.4 decisions are unchanged.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// MF4: a genuine signed reply for licence B, served to licence A, grants nothing
// in v1.5 on every online path. v1.4 keeps accepting it (unchanged, by ADR 0021).
func TestSignedReplyForAnotherKeyIsRefusedV15(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		want := !compatMode15()
		newPingStub(t, nil) // signs a reply for testLicenseKey
		ctx := context.Background()
		warn := func(string) {}

		if res, err := RefreshCache(ctx, otherLicenseKey); (err == nil && res.Valid) != want {
			t.Errorf("RefreshCache: %+v, %v, want granted=%v", res, err, want)
		}
		if res, _ := ValidateFull(ctx, otherLicenseKey); res.Valid != want {
			t.Errorf("ValidateFull: %+v, want valid=%v", res, want)
		}
		vr, _ := Validate(ctx, otherLicenseKey, &ValidatorOptions{WarnOnce: warn})
		if vr.CanProceed != want {
			t.Errorf("Validate: %+v, want proceed=%v", vr, want)
		}
		// The reply is still good for the licence it names.
		if res, err := RefreshCache(ctx, testLicenseKey); err != nil || !res.Valid {
			t.Errorf("a reply must still grant its own licence: %+v, %v", res, err)
		}
	})
}

// A signed reply with no jwt, a jwt of the wrong shape, or a jwt outside its
// window cannot be tied to the requesting licence now, so v1.5 refuses it.
func TestSignedReplyWithoutAUsableBindingIsRefusedV15(t *testing.T) {
	now := time.Now()
	cases := map[string]struct {
		body   func(m map[string]any)
		claims func(c map[string]any)
	}{
		"no jwt":                 {body: func(m map[string]any) { delete(m, "jwt") }},
		"jwt sub is another key": {claims: func(c map[string]any) { c["sub"] = HashKey(otherLicenseKey) }},
		"jwt expired": {claims: func(c map[string]any) {
			c["iat"], c["exp"] = now.Add(-48*time.Hour).Unix(), now.Add(-24*time.Hour).Unix()
		}},
		"jwt issued in future": {claims: func(c map[string]any) { c["iat"], c["exp"] = now.Add(2*time.Hour).Unix(), now.Add(26*time.Hour).Unix() }},
		"jwt garbage":          {body: func(m map[string]any) { m["jwt"] = "a.b.c" }},
	}
	for name, c := range cases {
		compattest.Both(t, func(t *testing.T) {
			want := !compatMode15()
			newPingStubWith(t, c.body, c.claims)
			res, err := RefreshCache(context.Background(), testLicenseKey)
			if (err == nil && res.Valid) != want {
				t.Errorf("%s: %+v, %v, want granted=%v", name, res, err, want)
			}
		})
	}
}

// ---- bundle replies ---------------------------------------------------

func bundleBody(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	now := time.Now().Unix()
	m := map[string]any{"valid": true, "tier": "owner", "bundle": "chat", "key_hash": HashKey(testLicenseKey),
		"issued_at": now, "expires_at": now + 3600}
	if mutate != nil {
		mutate(m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func serveSigned(t *testing.T, priv ed25519.PrivateKey, body []byte) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-NSelf-License-Sig", hex.EncodeToString(ed25519.Sign(priv, body)))
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LICENSE_PING_URL", srv.URL)
}

// MF5: a genuine signed bundle reply grants only the licence, bundle and window
// it names. Today's prod bundle reply names none of them and is unsigned, so
// v1.5 refuses it until ping signs the contract in the handoff.
func TestBundleReplyMustNameKeyBundleAndWindowV15(t *testing.T) {
	now := time.Now().Unix()
	cases := []struct {
		name   string
		mutate func(map[string]any)
		v14    bool // v1.4 still accepts a validly signed reply
		v15    bool
	}{
		{"bound reply", nil, true, true},
		{"reply for another bundle (task replayed for chat)", func(m map[string]any) { m["bundle"] = "task" }, true, false},
		{"main-path style reply without a bundle", func(m map[string]any) { delete(m, "bundle") }, true, false},
		{"reply for another licence key", func(m map[string]any) { m["key_hash"] = HashKey(otherLicenseKey) }, true, false},
		{"no key_hash", func(m map[string]any) { delete(m, "key_hash") }, true, false},
		{"no window", func(m map[string]any) { delete(m, "issued_at"); delete(m, "expires_at") }, true, false},
		{"expired window", func(m map[string]any) { m["issued_at"], m["expires_at"] = now-7200, now-3600 }, true, false},
		{"issued in the future", func(m map[string]any) { m["issued_at"], m["expires_at"] = now+7200, now+10800 }, true, false},
		{"window longer than 24 h", func(m map[string]any) { m["issued_at"], m["expires_at"] = now, now+90000 }, true, false},
		{"window ends before it starts", func(m map[string]any) { m["issued_at"], m["expires_at"] = now, now-1 }, true, false},
	}
	for _, c := range cases {
		compattest.Both(t, func(t *testing.T) {
			checkerCacheDir(t)
			priv := useTestPingKey(t)
			serveSigned(t, priv, bundleBody(t, c.mutate))
			want := c.v14
			if compatMode15() {
				want = c.v15
			}
			ok, err := BundleEntitled(context.Background(), testLicenseKey, "chat")
			if ok != want || (!want && err == nil) {
				t.Errorf("%s: granted=%v err=%v, want granted=%v", c.name, ok, err, want)
			}
		})
	}
}

// ---- MF6: freshness from the signed issued-at ------------------------

// restamped builds a genuine ping-signed cache entry whose JWT was issued
// `issuedAgo` ago while fetched_at is restamped to now (the replay attack).
func restamped(t *testing.T, issuedAgo time.Duration) *CacheEntry {
	t.Helper()
	stub := newPingStubWith(t, nil, func(c map[string]any) {
		c["iat"] = time.Now().Add(-issuedAgo).Unix()
		c["exp"] = time.Now().Add(-issuedAgo + 24*time.Hour).Unix()
	})
	var resp ValidateResponse
	if err := json.Unmarshal(stub.body, &resp); err != nil {
		t.Fatal(err)
	}
	resp.RawBody, resp.BodySig = string(stub.body), hex.EncodeToString(ed25519.Sign(stub.priv, stub.body))
	e := responseToCache(testLicenseKey, &resp)
	e.FetchedAt = time.Now().Unix()
	return e
}

func TestCacheAgeIsAnchoredOnSignedIssuedAtV15(t *testing.T) {
	cases := []struct {
		name      string
		issuedAgo time.Duration
		grantedV5 bool // v1.5 offline grant
	}{
		{"issued an hour ago", time.Hour, true},
		{"issued 4 days ago (warning window)", 4 * 24 * time.Hour, true},
		{"issued 8 days ago, fetched_at restamped", 8 * 24 * time.Hour, false},
		{"issued 90 days ago, fetched_at restamped", 90 * 24 * time.Hour, false},
	}
	for _, c := range cases {
		compattest.Both(t, func(t *testing.T) {
			withTempCachePath(t)
			checkerCacheDir(t)
			e := restamped(t, c.issuedAgo)
			seedCache(t, e)
			offline(t)
			t.Setenv("NSELF_LICENSE_FAIL_OPEN", "")
			want := true // v1.4 reads the restamped fetched_at: fresh
			if compatMode15() {
				want = c.grantedV5
				if !e.VerifySignature() {
					t.Fatal("precondition: the replayed entry is genuine and verifies")
				}
			}
			var ok bool
			var err error
			captureStderr(t, func() { ok, err = BundleEntitled(context.Background(), testLicenseKey, "chat") })
			if ok != want {
				t.Errorf("%s BundleEntitled: granted=%v err=%v, want %v", c.name, ok, err, want)
			}
			// Past the 7 d ceiling ValidateFull stays "valid" but read-only: WriteAllowed is the grant.
			if res, _ := ValidateFull(context.Background(), testLicenseKey); res.WriteAllowed != want {
				t.Errorf("%s ValidateFull: %+v, want write allowed=%v", c.name, res, want)
			}
			if !compatMode15() {
				return // v1.4 Validate checks the legacy signature, which no genuine entry carries
			}
			warn := func(string) {}
			vr, _ := Validate(context.Background(), testLicenseKey, &ValidatorOptions{PingURL: PingURL(), WarnOnce: warn})
			if vr.CanProceed != want {
				t.Errorf("%s Validate: %+v, want proceed=%v", c.name, vr, want)
			}
		})
	}
}

func TestAgeAtTakesTheOlderOfFetchedAndSignedIssuedAt(t *testing.T) {
	compattest.Set(t, true)
	now := time.Now()
	old := restamped(t, 3*24*time.Hour)
	if got := old.AgeAt(now); got < 3*24*time.Hour-time.Minute || got > 3*24*time.Hour+time.Minute {
		t.Errorf("restamped entry: AgeAt = %v, want ~72h from the signed iat", got)
	}
	if got := old.CacheAge(); got < 71*time.Hour || got > 73*time.Hour {
		t.Errorf("CacheAge = %v, want ~72h", got)
	}
	// fetched_at older than iat: the older one wins too.
	fresh := restamped(t, time.Minute)
	fresh.FetchedAt = now.Add(-5 * time.Hour).Unix()
	if got := fresh.AgeAt(now); got < 5*time.Hour-time.Minute || got > 5*time.Hour+time.Minute {
		t.Errorf("AgeAt = %v, want ~5h from fetched_at", got)
	}
	// No signed body: fetched_at alone.
	bare := &CacheEntry{FetchedAt: now.Add(-2 * time.Hour).Unix()}
	if got := bare.AgeAt(now); got < 2*time.Hour || got > 2*time.Hour+time.Second {
		t.Errorf("unsigned entry AgeAt = %v, want 2h", got)
	}
	// v1.4 ignores the signed iat.
	compattest.Set(t, false)
	if got := old.AgeAt(now); got > time.Minute {
		t.Errorf("v1.4 AgeAt = %v, want the fetched_at age", got)
	}
}

func TestSignedReplyErrorsNameTheProblem(t *testing.T) {
	r := &ValidateResponse{Valid: true}
	if err := replyBoundToKey(r, testLicenseKey, time.Now()); err == nil || !strings.Contains(err.Error(), "no licence jwt") {
		t.Errorf("missing jwt: %v", err)
	}
	b := &bundleValidateResponse{Valid: true, Bundle: "chat"}
	if err := bundleReplyBound(b, testLicenseKey, "task", time.Now()); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%q", "task")) {
		t.Errorf("wrong bundle: %v", err)
	}
}

// The window arithmetic at its exact edges (now is injected).
func TestReplyWindowEdges(t *testing.T) {
	priv := useTestPingKey(t)
	now := time.Unix(1_800_000_000, 0)
	n := now.Unix()
	jwtFor := func(iat, exp int64) *ValidateResponse {
		c := goodClaims()
		c["sub"], c["iat"], c["exp"] = HashKey(testLicenseKey), iat, exp
		return &ValidateResponse{Valid: true, JWT: signEdDSA(t, priv, goodHeader(), c)}
	}
	for name, c := range map[string]struct {
		iat, exp int64
		ok       bool
	}{
		"issued now, 1 h":                  {n, n + 3600, true},
		"issued exactly at the skew limit": {n + replySkewSec, n + 3600, true},
		"issued 1 s past the skew limit":   {n + replySkewSec + 1, n + 7200, false},
		"expires 1 s from now":             {n - 100, n + 1, true},
		"expires exactly now":              {n - 100, n, false},
		"expired":                          {n - 7200, n - 1, false},
	} {
		if err := replyBoundToKey(jwtFor(c.iat, c.exp), testLicenseKey, now); (err == nil) != c.ok {
			t.Errorf("jwt %s: err=%v, want ok=%v", name, err, c.ok)
		}
	}
	if err := replyBoundToKey(jwtFor(n, n+3600), otherLicenseKey, now); err == nil {
		t.Error("a jwt for another key must be refused")
	}
	bundle := func(issued, expires int64) error {
		return bundleReplyBound(&bundleValidateResponse{Valid: true, Bundle: "chat", KeyHash: HashKey(testLicenseKey),
			IssuedAt: issued, ExpiresAt: expires}, testLicenseKey, "chat", now)
	}
	day := int64(24 * 60 * 60)
	for name, c := range map[string]struct {
		issued, expires int64
		ok              bool
	}{
		"issued now, 1 h":                  {n, n + 3600, true},
		"exactly 24 h long":                {n, n + day, true},
		"24 h and 1 s long":                {n, n + day + 1, false},
		"issued at the skew limit":         {n + replySkewSec, n + 3600, true},
		"issued 1 s past the skew limit":   {n + replySkewSec + 1, n + 3600, false},
		"expires 1 s from now":             {n - 100, n + 1, true},
		"expires exactly now":              {n - 100, n, false},
		"issued_at zero, expiry in future": {0, n + 100, false},
		"issued_at negative":               {-5, n + 100, false},
		"empty window (expires == issued)": {n + 10, n + 10, false},
		"expires before issued":            {n + 10, n + 5, false},
	} {
		if err := bundle(c.issued, c.expires); (err == nil) != c.ok {
			t.Errorf("bundle %s: err=%v, want ok=%v", name, err, c.ok)
		}
	}
}

// Validate judges a reply's window by its own clock, so a test clock (or a
// skewed machine) is honoured, and the zero options still use the wall clock.
func TestValidatorUsesItsClockForTheReplyWindow(t *testing.T) {
	compattest.Set(t, true)
	newPingStub(t, nil)
	past := fixedClock{t: time.Now().Add(-72 * time.Hour)}
	warn := func(string) {}
	vr, _ := Validate(context.Background(), testLicenseKey, &ValidatorOptions{Clock: past, WarnOnce: warn})
	if vr.CanProceed {
		t.Errorf("a reply issued 72 h after the validator's clock must be refused: %+v", vr)
	}
	vr, _ = Validate(context.Background(), testLicenseKey, &ValidatorOptions{Clock: fixedClock{t: time.Now()}, WarnOnce: warn})
	if !vr.CanProceed {
		t.Errorf("a reply inside the window must be accepted: %+v", vr)
	}
	for _, o := range []*ValidatorOptions{nil, {}} {
		if d := time.Since(clockNow(o)); d < 0 || d > time.Minute {
			t.Errorf("clockNow(%v) is %v from the wall clock", o, d)
		}
	}
	if got := clockNow(&ValidatorOptions{Clock: past}); !got.Equal(past.t) {
		t.Errorf("clockNow ignored the configured clock: %v", got)
	}
}
