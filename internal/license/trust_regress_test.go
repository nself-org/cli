package license

// trust_regress_test.go: the P7-PLUG-63 review holes. One rule everywhere in
// v1.5: no licence decision from bytes that did not verify under PingKeys.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
)

const otherLicenseKey = "nself_owner_someoneelseskeyvalue00"

// offline points ping at a server that drops every connection.
func offline(t *testing.T) {
	t.Helper()
	t.Setenv("LICENSE_PING_URL", newRefusingServer(t).URL)
}

func handWrittenCache(t *testing.T, key string) {
	t.Helper()
	seedCache(t, &CacheEntry{
		KeyHash: HashKey(key), Tier: "owner", PluginsAllowed: []string{"bundle:chat", "*"},
		FetchedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(30 * 24 * time.Hour).Unix(),
	})
}

// MF1: a hand-written (unsigned) cache never unlocks a licensed bundle in v1.5
// on any fallback path; v1.4 behaviour is unchanged.
func TestHandWrittenCacheNeverUnlocksV15(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		want := !compatMode15()
		checkerCacheDir(t)
		withTempCachePath(t)
		offline(t)
		handWrittenCache(t, testLicenseKey)
		ctx := context.Background()

		t.Setenv("NSELF_LICENSE_FAIL_OPEN", "")
		ok, err := BundleEntitled(ctx, testLicenseKey, "chat")
		if ok != want {
			t.Errorf("grace path: granted=%v err=%v, want granted=%v", ok, err, want)
		}
		t.Setenv("NSELF_LICENSE_FAIL_OPEN", "1")
		ok, err = BundleEntitled(ctx, testLicenseKey, "chat")
		if ok != want {
			t.Errorf("fail-open path: granted=%v err=%v, want granted=%v", ok, err, want)
		}
		res, err := ValidateFull(ctx, testLicenseKey)
		if err != nil || res.Valid != want {
			t.Errorf("ValidateFull fallback: %+v, %v, want valid=%v", res, err, want)
		}
		if !compatMode15() {
			return
		}
		// Validate (the fail-open validator) refuses it too, with verification on.
		warn, _, _ := captureWarn()
		vr, _ := Validate(ctx, testLicenseKey, &ValidatorOptions{PingURL: PingURL(), WarnOnce: warn})
		if vr.CanProceed {
			t.Errorf("Validate fail-open granted a hand-written cache: %+v", vr)
		}
	})
}

// A genuine signed cache still works offline in v1.5: the fix refuses forgeries, not ping.
func TestGenuineSignedCacheStillGrantsOffline(t *testing.T) {
	newPingStub(t, nil)
	withTempCachePath(t)
	compattest.Set(t, true)
	if _, entry := validateAndRead(t); !entry.VerifySignature() {
		t.Fatal("precondition: ping-signed cache verifies")
	}
	offline(t)
	for _, failOpen := range []string{"", "1"} {
		t.Setenv("NSELF_LICENSE_FAIL_OPEN", failOpen)
		if ok, err := BundleEntitled(context.Background(), testLicenseKey, "chat"); !ok || err != nil {
			t.Errorf("fail-open=%q: genuine signed cache refused: %v", failOpen, err)
		}
	}
	if res, err := ValidateFull(context.Background(), testLicenseKey); err != nil || !res.Valid || !res.FromCache {
		t.Errorf("ValidateFull from a genuine cache: %+v, %v", res, err)
	}
}

// MF2: licence A's genuine signed cache is bound to A. Presented for B, with the
// cache's own jwt copy blanked or swapped, it never verifies and never grants.
func TestSignedCacheIsBoundToItsLicence(t *testing.T) {
	stub := newPingStub(t, nil)
	withTempCachePath(t)
	compattest.Set(t, true)
	_, a := validateAndRead(t)
	if !a.VerifySignature() {
		t.Fatal("precondition: A's own cache verifies")
	}
	forBlanked := func(e *CacheEntry) { e.KeyHash, e.JWT = HashKey(otherLicenseKey), "" }
	forSwapped := func(e *CacheEntry) {
		claims := goodClaims()
		claims["sub"] = HashKey(otherLicenseKey)
		e.KeyHash, e.JWT = HashKey(otherLicenseKey), signEdDSA(t, stub.priv, goodHeader(), claims)
	}
	blankedForA := func(e *CacheEntry) { e.JWT = "" }
	for name, mut := range map[string]func(*CacheEntry){
		"A's body, B's key hash, jwt blanked":                 forBlanked,
		"A's body, B's key hash, jwt swapped for one for B":   forSwapped,
		"A's own cache with its jwt copy blanked":             blankedForA,
		"A's own cache with the jwt copy swapped for a stale": func(e *CacheEntry) { e.JWT += "x" },
	} {
		c := *a
		mut(&c)
		if c.VerifySignature() {
			t.Errorf("%s: verified", name)
		}
	}
	// End to end: B holds A's cache file and ping is unreachable.
	c := *a
	forBlanked(&c)
	seedCache(t, &c)
	offline(t)
	for _, failOpen := range []string{"", "1"} {
		t.Setenv("NSELF_LICENSE_FAIL_OPEN", failOpen)
		if ok, _ := BundleEntitled(context.Background(), otherLicenseKey, "chat"); ok {
			t.Errorf("fail-open=%q: licence B unlocked by licence A's signed cache", failOpen)
		}
	}
	if res, _ := ValidateFull(context.Background(), otherLicenseKey); res.Valid {
		t.Error("ValidateFull granted B from A's cache")
	}
}

// MF3: the online bundle check counts only a body whose signature verifies
// under PingKeys. A present-but-invalid signature is refused in both modes; a
// missing one from v1.5.
func TestBundleEntitledOnlineNeedsAVerifiedSignature(t *testing.T) {
	now := time.Now().Unix()
	body := []byte(fmt.Sprintf(`{"valid":true,"tier":"owner","bundle":"chat","key_hash":%q,"issued_at":%d,"expires_at":%d}`,
		HashKey(testLicenseKey), now, now+3600))
	_, stranger, _ := ed25519.GenerateKey(nil)
	serve := func(t *testing.T, sig func(priv ed25519.PrivateKey) (string, bool)) {
		priv := useTestPingKey(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s, ok := sig(priv); ok {
				w.Header().Set("X-NSelf-License-Sig", s)
			}
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		t.Setenv("LICENSE_PING_URL", srv.URL)
	}
	hexSig := func(k ed25519.PrivateKey, b []byte) string { return hex.EncodeToString(ed25519.Sign(k, b)) }
	cases := []struct {
		name       string
		sig        func(ed25519.PrivateKey) (string, bool)
		grantedV14 bool
		grantedV15 bool
	}{
		{"no signature", func(ed25519.PrivateKey) (string, bool) { return "", false }, true, false},
		{"garbage signature", func(ed25519.PrivateKey) (string, bool) { return "deadbeef", true }, false, false},
		{"not hex", func(ed25519.PrivateKey) (string, bool) { return "zz", true }, false, false},
		{"signed by an unknown key", func(ed25519.PrivateKey) (string, bool) { return hexSig(stranger, body), true }, false, false},
		{"signature over other bytes", func(k ed25519.PrivateKey) (string, bool) { return hexSig(k, []byte(`{"valid":false}`)), true }, false, false},
		{"genuine signature", func(k ed25519.PrivateKey) (string, bool) { return hexSig(k, body), true }, true, true},
	}
	for _, c := range cases {
		compattest.Both(t, func(t *testing.T) {
			checkerCacheDir(t)
			serve(t, c.sig)
			want := c.grantedV14
			if compatMode15() {
				want = c.grantedV15
			}
			ok, err := BundleEntitled(context.Background(), testLicenseKey, "chat")
			if ok != want || (!want && err == nil) {
				t.Errorf("%s: granted=%v err=%v, want granted=%v", c.name, ok, err, want)
			}
		})
	}
}
