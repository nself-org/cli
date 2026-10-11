package license

// trust_test.go: D-0233. A verified cache refused because the clock is behind
// the evidence gets clock text that says what to do; a cache that does not
// verify keeps the tamper text. v1.4 has no clock check, so neither fires there.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
)

const clockHint = "system clock is behind"

func bundleErr(t *testing.T) error {
	t.Helper()
	var err error
	captureStderr(t, func() { _, err = BundleEntitled(context.Background(), testLicenseKey, "chat") })
	return err
}

func TestClockRefusalSaysClockNotTamperV15(t *testing.T) {
	compattest.Set(t, true)
	for _, failOpen := range []string{"", "1"} {
		// signed iat 30 days ahead of the clock: a genuine cache read on a rolled-back clock
		e := signedEntry(t, 60*24*time.Hour, -30*24*time.Hour)
		offlineWith(t, e, failOpen)
		err := bundleErr(t)
		if err == nil || !strings.Contains(err.Error(), clockHint) || strings.Contains(err.Error(), "not signed by ping") {
			t.Errorf("fail-open=%q: BundleEntitled err = %v, want the clock text", failOpen, err)
		}
		if err != nil && !strings.Contains(err.Error(), "nself license import") {
			t.Errorf("fail-open=%q: clock text must name the way out: %v", failOpen, err)
		}
		res, _ := ValidateFull(context.Background(), testLicenseKey)
		if res.Valid || !strings.Contains(res.Message, clockHint) || strings.Contains(res.Message, "not signed by ping") {
			t.Errorf("fail-open=%q: ValidateFull = %+v, want the clock text", failOpen, res)
		}
		vr, _ := Validate(context.Background(), testLicenseKey, &ValidatorOptions{PingURL: PingURL(), WarnOnce: func(string) {}})
		if vr.CanProceed || !strings.Contains(vr.Reason, clockHint) || strings.Contains(vr.Reason, "tampered") {
			t.Errorf("fail-open=%q: Validate = %+v, want the clock reason", failOpen, vr)
		}
	}
}

func TestTamperedCacheKeepsTamperTextV15(t *testing.T) {
	compattest.Set(t, true)
	for _, failOpen := range []string{"", "1"} {
		// clock is also behind, but the signature is checked first
		e := signedEntry(t, 60*24*time.Hour, -30*24*time.Hour)
		e.Tier = "owner"
		offlineWith(t, e, failOpen)
		err := bundleErr(t)
		if err == nil || !strings.Contains(err.Error(), "not signed by ping") || strings.Contains(err.Error(), clockHint) {
			t.Errorf("fail-open=%q: BundleEntitled err = %v, want the tamper text", failOpen, err)
		}
		res, _ := ValidateFull(context.Background(), testLicenseKey)
		if res.Valid || !strings.Contains(res.Message, "not signed by ping") {
			t.Errorf("fail-open=%q: ValidateFull = %+v, want the tamper text", failOpen, res)
		}
		vr, _ := Validate(context.Background(), testLicenseKey, &ValidatorOptions{PingURL: PingURL(), WarnOnce: func(string) {}})
		if vr.CanProceed || !strings.Contains(vr.Reason, "tampered") || strings.Contains(vr.Reason, clockHint) {
			t.Errorf("fail-open=%q: Validate = %+v, want the tamper reason", failOpen, vr)
		}
	}
}

func TestEntryTrustAtOrderV15(t *testing.T) {
	compattest.Set(t, true)
	// one signed entry per test: each stub installs its own ping key
	ahead := signedEntry(t, 60*24*time.Hour, -30*24*time.Hour)
	withTempCachePath(t)
	iat, _ := ahead.signedIssuedAt()
	if err := entryTrustAt(ahead, time.Unix(iat, 0)); err != nil {
		t.Errorf("genuine cache at its iat: %v", err)
	}
	if err := entryTrustAt(ahead, time.Now()); !errors.Is(err, errClockBehind) {
		t.Errorf("rolled-back clock: err = %v, want errClockBehind", err)
	}
	ahead.Tier = "owner"
	if err := entryTrustAt(ahead, time.Now()); !errors.Is(err, errCacheUnsigned) {
		t.Errorf("altered cache on a rolled-back clock: err = %v, want errCacheUnsigned", err)
	}
}

func TestCacheTrustRaisesMarkOnlyWhenTrustedV15(t *testing.T) {
	compattest.Set(t, true)
	good := signedEntry(t, 30*24*time.Hour, time.Hour)
	altered := *good
	bad := &altered
	bad.Tier = "owner"
	withTempCachePath(t)
	checkerCacheDir(t)
	if err := cacheTrust(good); err != nil {
		t.Fatalf("cacheTrust(good) = %v", err)
	}
	if readTrustedTime().Before(time.Now().Add(-time.Minute)) {
		t.Error("a trusted decision must raise the mark to now")
	}
	writeTrustedTime(time.Unix(1, 0))
	if err := cacheTrust(bad); !errors.Is(err, errCacheUnsigned) {
		t.Errorf("cacheTrust(altered) = %v", err)
	}
	if got := readTrustedTime(); got.Unix() != 1 {
		t.Errorf("a refused decision must not move the mark, got %v", got)
	}
}

func TestCacheTrustV14TrustsEverything(t *testing.T) {
	compattest.Set(t, false)
	withTempCachePath(t)
	checkerCacheDir(t)
	e := &CacheEntry{KeyHash: HashKey(testLicenseKey), Tier: "owner"}
	if err := cacheTrust(e); err != nil {
		t.Errorf("v1.4 cacheTrust = %v, want nil", err)
	}
	if !readTrustedTime().IsZero() {
		t.Error("v1.4 must not write a trusted-time mark")
	}
}

func TestRefusalTextHelpers(t *testing.T) {
	if m := cacheRefusalMessage(errClockBehind); !strings.Contains(m, clockHint) || !strings.HasPrefix(m, "License cache refused: ") {
		t.Errorf("clock message = %q", m)
	}
	if m := cacheRefusalMessage(errCacheUnsigned); !strings.Contains(m, "not signed by ping") {
		t.Errorf("unsigned message = %q", m)
	}
	if r := validatorRefusalReason(errClockBehind); !strings.Contains(r, clockHint) || !strings.HasSuffix(r, "refusing fail-open") {
		t.Errorf("clock reason = %q", r)
	}
	if r := validatorRefusalReason(errCacheUnsigned); !strings.Contains(r, "tampered") {
		t.Errorf("unsigned reason = %q", r)
	}
}
