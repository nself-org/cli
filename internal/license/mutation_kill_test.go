package license

// mutation_kill_test.go: behaviour tests aimed at the mutants gremlins left
// alive in the P7-PLUG-63 files (validate, validator, validator_validate,
// checker, keys, jwt, cache_entry). Each asserts an observable decision, not
// a line: request timeout, stderr warnings, expiry mapping, grace wording.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// deadlineRT records the context deadline of the request it is handed, then
// fails the request: nothing leaves the machine.
type deadlineRT struct {
	left *time.Duration
	seen *bool
}

func (d deadlineRT) RoundTrip(r *http.Request) (*http.Response, error) {
	if dl, ok := r.Context().Deadline(); ok {
		*d.left, *d.seen = time.Until(dl), true
	}
	return nil, errors.New("stub transport: no network")
}

// clientTimeout runs call with http.DefaultTransport replaced and reports the
// deadline the default client armed on the request (its Timeout).
func clientTimeout(t *testing.T, call func()) (time.Duration, bool) {
	t.Helper()
	var left time.Duration
	var seen bool
	orig := http.DefaultTransport
	http.DefaultTransport = deadlineRT{&left, &seen}
	t.Cleanup(func() { http.DefaultTransport = orig })
	call()
	return left, seen
}

func want30s(t *testing.T, name string, left time.Duration, seen bool) {
	t.Helper()
	if !seen {
		t.Fatalf("%s: the HTTP client armed no timeout (a hung ping would hang the CLI)", name)
	}
	if left <= 29*time.Second || left > 30*time.Second {
		t.Errorf("%s: client timeout = %v, want 30s", name, left)
	}
}

func TestRemoteCallsCarryA30sTimeout(t *testing.T) {
	redirectCache(t)
	t.Setenv("NSELF_LICENSE_FAIL_OPEN", "")
	t.Setenv("LICENSE_PING_URL", "http://ping.invalid")
	ctx := context.Background()

	left, seen := clientTimeout(t, func() { _, _ = ValidateFull(ctx, testLicenseKey) })
	want30s(t, "ValidateFull", left, seen)

	left, seen = clientTimeout(t, func() { _, _ = BundleEntitled(ctx, testLicenseKey, "chat") })
	want30s(t, "BundleEntitled", left, seen)

	left, seen = clientTimeout(t, func() {
		_, _ = Validate(ctx, testLicenseKey, &ValidatorOptions{PingURL: "http://ping.invalid", WarnOnce: func(string) {}})
	})
	want30s(t, "Validate", left, seen)
}

// unwritableCache points the cache at a path below a regular file, so every
// WriteCache fails.
func unwritableCache(t *testing.T) {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(blocker, "sub", "license.json"))
	if err := WriteCache(&CacheEntry{KeyHash: "x"}); err == nil {
		t.Fatal("precondition: the cache path must be unwritable")
	}
}

const cacheWarning = "could not update license cache"

func TestValidateFullWarnsOnlyWhenTheCacheWriteFails(t *testing.T) {
	newPingStub(t, nil)
	var res *ValidationResult
	out := captureStderr(t, func() { res, _ = ValidateFull(context.Background(), testLicenseKey) })
	if res == nil || !res.Valid || strings.Contains(out, cacheWarning) {
		t.Fatalf("a good write must be silent: res=%+v stderr=%q", res, out)
	}
	if want := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC); !res.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want the server's expires_at %v", res.ExpiresAt, want)
	}

	unwritableCache(t)
	out = captureStderr(t, func() { res, _ = ValidateFull(context.Background(), testLicenseKey) })
	if res == nil || !res.Valid {
		t.Fatalf("a failed cache write is non-fatal: %+v", res)
	}
	if !strings.Contains(out, cacheWarning) {
		t.Errorf("a failed cache write must warn on stderr, got %q", out)
	}
}

func TestRefreshCacheMapsServerExpiry(t *testing.T) {
	newPingStub(t, nil)
	res, err := RefreshCache(context.Background(), testLicenseKey)
	if err != nil || !res.Valid {
		t.Fatalf("RefreshCache: %+v, %v", res, err)
	}
	if want := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC); !res.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", res.ExpiresAt, want)
	}
	newPingStub(t, func(m map[string]any) { delete(m, "expires_at") })
	res, err = RefreshCache(context.Background(), testLicenseKey)
	if err != nil || !res.Valid || !res.ExpiresAt.IsZero() {
		t.Errorf("no expires_at must leave ExpiresAt zero: %+v, %v", res, err)
	}
}

func TestValidatorWarnsOnlyWhenTheCacheWriteFails(t *testing.T) {
	newPingStub(t, nil)
	opts := func() *ValidatorOptions { return &ValidatorOptions{WarnOnce: func(string) {}} }
	var res *ValidatorResult
	out := captureStderr(t, func() { res, _ = Validate(context.Background(), testLicenseKey, opts()) })
	if res == nil || res.Status != StatusValid || strings.Contains(out, cacheWarning) {
		t.Fatalf("a good write must be silent: res=%+v stderr=%q", res, out)
	}
	unwritableCache(t)
	out = captureStderr(t, func() { res, _ = Validate(context.Background(), testLicenseKey, opts()) })
	if res == nil || res.Status != StatusValid {
		t.Fatalf("a failed cache write is non-fatal: %+v", res)
	}
	if !strings.Contains(out, cacheWarning) {
		t.Errorf("a failed cache write must warn on stderr, got %q", out)
	}
}

// A signed entry with no server expiry (expires_at 0) is not expired: only a
// positive expiry in the past expires a licence.
func TestValidatorZeroExpiryIsNotExpired(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	e := makeEntry(now, time.Hour, 0, "nself_pro_zeroexpiry0000000000000000000")
	e.ExpiresAt = 0
	seedCache(t, e)
	warn, _, _ := captureWarn()
	res, err := Validate(context.Background(), "nself_pro_zeroexpiry0000000000000000000",
		silentOpts(now, errDoer{errors.New("offline")}, warn))
	if err != nil || res.Status != StatusFailOpen || !res.CanProceed {
		t.Errorf("zero expiry must fall through to the TTL window: %+v, %v", res, err)
	}
	// And a positive expiry exactly in the past does expire it.
	e.ExpiresAt = now.Add(-time.Second).Unix()
	seedCache(t, e)
	res, _ = Validate(context.Background(), "nself_pro_zeroexpiry0000000000000000000",
		silentOpts(now, errDoer{errors.New("offline")}, warn))
	if res.Status != StatusExpired || res.CanProceed {
		t.Errorf("a past expiry must expire the licence: %+v", res)
	}
}

// The hard-window warning tells the user how long is left, not how long it has been.
func TestValidatorHardWindowWarningNamesTimeLeft(t *testing.T) {
	redirectCache(t)
	const key = "nself_pro_hardwindow00000000000000000000"
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	age := 5 * 24 * time.Hour
	seedCache(t, makeEntry(now, age, 30*24*time.Hour, key))
	warn, msgs, _ := captureWarn()
	res, err := Validate(context.Background(), key, silentOpts(now, errDoer{errors.New("offline")}, warn))
	if err != nil || res.Status != StatusFailOpen || res.WarnMessage == "" {
		t.Fatalf("want fail-open with a warning: %+v, %v", res, err)
	}
	want := "offline for " + formatDuration(age) + ". Connect to the internet within " +
		formatDuration(FailOpenHardTTL-age) + " "
	if len(*msgs) != 1 || !strings.Contains((*msgs)[0], want) || res.WarnMessage != (*msgs)[0] {
		t.Errorf("warning = %v, want it to contain %q", *msgs, want)
	}
	if res.CacheAge != age {
		t.Errorf("CacheAge = %v, want %v", res.CacheAge, age)
	}
}

func TestEmitGraceWarningCountsDownAndClampsAtZero(t *testing.T) {
	age := 5 * 24 * time.Hour
	out := captureStderr(t, func() { emitGraceWarning("chat", GraceCheckResult{CacheAge: age}) })
	want := "(last validated " + formatDuration(age) + " ago). Offline grace period expires in " +
		formatDuration(GraceHardThreshold-age) + "."
	if !strings.Contains(out, want) || !strings.Contains(out, `"chat"`) {
		t.Errorf("warning = %q, want it to contain %q", out, want)
	}
	over := GraceHardThreshold + 3*time.Hour
	out = captureStderr(t, func() { emitGraceWarning("chat", GraceCheckResult{CacheAge: over}) })
	want = "expires in " + formatDuration(0) + "."
	if !strings.Contains(out, want) {
		t.Errorf("past the ceiling the warning must say %q, got %q", want, out)
	}
}

// With no environment variable set, CollectLicenseKey falls back to the stored key file.
func TestCollectLicenseKeyFallsBackToStoredFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY_OWNER", "")
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	dir := filepath.Join(home, licenseDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := CollectLicenseKey(); got != "" {
		t.Fatalf("no key anywhere: got %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, keyFile), []byte("nself_plus_storedonly0000000000000001\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := CollectLicenseKey(); got != "nself_plus_storedonly0000000000000001" {
		t.Errorf("CollectLicenseKey = %q, want the stored key", got)
	}
}

func TestCollectLicenseKeyOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY_OWNER", "")
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "nself_plus_envkeyenvkeyenvkeyenvkey")
	if got := CollectLicenseKey(); got != "nself_plus_envkeyenvkeyenvkeyenvkey" {
		t.Errorf("CollectLicenseKey = %q, want the NSELF_PLUGIN_LICENSE_KEY value", got)
	}
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY_OWNER", "nself_owner_ownerownerownerownerown")
	if got := CollectLicenseKey(); got != "nself_owner_ownerownerownerownerown" {
		t.Errorf("the owner key must win, got %q", got)
	}
}

// ---- keys.go -----------------------------------------------------------

func TestCollectLicenseKeysReadsEveryNumberedVariableAndTheKeyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	for i := 1; i <= 11; i++ {
		t.Setenv(fmt.Sprintf("NSELF_LICENSE_KEY_%d", i), "")
	}
	t.Setenv("NSELF_LICENSE_KEY_1", "nself_plus_numberedone00000000000001")
	t.Setenv("NSELF_LICENSE_KEY_10", "nself_plus_numberedten000000000010")
	t.Setenv("NSELF_LICENSE_KEY_11", "nself_plus_numberedeleven00000011")
	dir := filepath.Join(home, licenseDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, keyFile), []byte("nself_plus_storedfilekey0000000000001\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(CollectLicenseKeys(), ",")
	for _, w := range []string{"numberedone", "numberedten", "storedfilekey"} {
		if !strings.Contains(got, w) {
			t.Errorf("CollectLicenseKeys = %q, missing %s", got, w)
		}
	}
	if strings.Contains(got, "numberedeleven") {
		t.Errorf("only NSELF_LICENSE_KEY_1..10 are read, got %q", got)
	}
}

func TestStoredKeyFilesOneToTenAreCollected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, licenseDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, key string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(key), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("key", "nself_plus_filezero0000000000000000")
	write("key.2", "nself_plus_filetwo00000000000000000")
	write("key.10", "nself_plus_fileten00000000000000000")
	write("key.11", "nself_plus_fileeleven000000000000000")
	got := strings.Join(GetAllStoredKeys(), ",")
	for _, w := range []string{"filezero", "filetwo", "fileten"} {
		if !strings.Contains(got, w) {
			t.Errorf("GetAllStoredKeys = %q, missing %s", got, w)
		}
	}
	if strings.Contains(got, "fileeleven") {
		t.Errorf("key.11 is beyond the ten slots, got %q", got)
	}
}

func TestValidateKeyFormatLengthBoundary(t *testing.T) {
	at := "nself_plus_" + strings.Repeat("a", minKeyLength-len("nself_plus_"))
	if len(at) != minKeyLength {
		t.Fatalf("test key is %d chars, want %d", len(at), minKeyLength)
	}
	if err := ValidateKeyFormat(at); err != nil {
		t.Errorf("a key of exactly %d chars with a known prefix must pass: %v", minKeyLength, err)
	}
	if err := ValidateKeyFormat(at[:len(at)-1]); err == nil {
		t.Error("a key one char short must be rejected")
	}
}

// ---- jwt.go ------------------------------------------------------------

func TestParseLicenseJWTAudienceList(t *testing.T) {
	priv := useTestPingKey(t)
	parse := func(aud any) error {
		c := goodClaims()
		c["aud"] = aud
		_, err := ParseLicenseJWT(signEdDSA(t, priv, goodHeader(), c))
		return err
	}
	for name, aud := range map[string]any{
		"list with nself-cli last":  []string{"x", "nself-cli"},
		"list with nself-cli first": []string{"nself-cli", "x"},
		"single entry list":         []string{"nself-cli"},
	} {
		if err := parse(aud); err != nil {
			t.Errorf("%s must be accepted: %v", name, err)
		}
	}
	for name, aud := range map[string]any{
		"list without nself-cli": []string{"x", "y"},
		"single other entry":     []string{"x"},
		"empty list":             []string{},
	} {
		if err := parse(aud); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
}

// ---- cache_entry.go: the v1.4 legacy signature --------------------------

func TestVerifyLegacyKeyIDHandling(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	setPingKeys(t, PingKey{ID: "8", Public: other}, PingKey{ID: "7", Public: pub})
	mk := func(keyID int) *CacheEntry {
		e := &CacheEntry{KeyHash: HashKey(testLicenseKey), Tier: "plus", FetchedAt: 100, ExpiresAt: 200,
			PluginsAllowed: []string{"b", "a"}, SignatureKeyID: keyID}
		e.Signature = hex.EncodeToString(ed25519.Sign(priv, e.signablePayload()))
		return e
	}
	for _, id := range []int{0, 7, 8, 9999} { // no id, right id, wrong id, unknown id (rotation window)
		if !mk(id).verifyLegacy() {
			t.Errorf("signature_key_id %d: a genuine legacy signature must verify", id)
		}
	}
	forged := mk(0)
	forged.Tier = "owner"
	bad := []*CacheEntry{forged}
	for _, sig := range []string{"", "zz", strings.Repeat("00", ed25519.SignatureSize)} {
		e := mk(0)
		e.Signature = sig
		bad = append(bad, e)
		e2 := mk(7)
		e2.Signature = sig
		bad = append(bad, e2)
	}
	for i, e := range bad {
		if e.verifyLegacy() {
			t.Errorf("forged legacy entry %d verified", i)
		}
	}
}

// ---- validator.go: how each HTTP status is classified ---------------------

func TestTryRemoteStatusClasses(t *testing.T) {
	opts := func(status int, body string) *ValidatorOptions {
		return &ValidatorOptions{HTTPClient: statusDoer{status: status, body: body}, PingURL: "http://ping.invalid", SkipSignatureVerify: true}
	}
	for status, want := range map[int]remoteOutcome{
		200: remoteOK,
		400: remoteAuthFail, 401: remoteAuthFail, 403: remoteAuthFail, 404: remoteAuthFail, 429: remoteAuthFail, 499: remoteAuthFail,
		500: remoteTransientFail, 502: remoteTransientFail, 503: remoteTransientFail,
	} {
		resp, got, err := tryRemote(context.Background(), testLicenseKey, opts(status, `{"valid":true,"tier":"plus"}`))
		if got != want {
			t.Errorf("status %d: outcome %v, want %v", status, got, want)
		}
		if (status == 200) != (err == nil && resp != nil) {
			t.Errorf("status %d: resp=%v err=%v", status, resp, err)
		}
	}
	// A 200 that is not JSON is transient (a server bug must not lock the user out).
	if _, got, err := tryRemote(context.Background(), testLicenseKey, opts(200, "<html>")); got != remoteTransientFail || err == nil {
		t.Errorf("200 with a non-JSON body: outcome %v err %v, want transient failure", got, err)
	}
	// With verification on, an unsigned 200 is never read as a licence.
	unsigned := &ValidatorOptions{HTTPClient: statusDoer{status: 200, body: `{"valid":true,"tier":"owner"}`}, PingURL: "http://ping.invalid"}
	if resp, got, err := tryRemote(context.Background(), testLicenseKey, unsigned); got == remoteOK || resp != nil || err == nil {
		t.Errorf("an unsigned 200 was accepted: %v %v %v", resp, got, err)
	}
}

// The offline window is inclusive: exactly FailOpenHardTTL still opens (with a
// warning), one second more is closed.
func TestValidatorHardTTLExactBoundary(t *testing.T) {
	redirectCache(t)
	const key = "nself_pro_boundary0000000000000000000000"
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	offline := errDoer{errors.New("offline")}
	run := func(age time.Duration) (*ValidatorResult, int) {
		seedCache(t, makeEntry(now, age, 365*24*time.Hour, key))
		warn, msgs, _ := captureWarn()
		res, err := Validate(context.Background(), key, silentOpts(now, offline, warn))
		if err != nil {
			t.Fatal(err)
		}
		return res, len(*msgs)
	}
	res, warned := run(FailOpenHardTTL)
	if res.Status != StatusFailOpen || !res.CanProceed || warned != 1 {
		t.Errorf("age == hard TTL: %+v warned=%d, want fail-open with one warning", res, warned)
	}
	res, warned = run(FailOpenHardTTL + time.Second)
	if res.Status != StatusFailClosed || res.CanProceed || warned != 0 {
		t.Errorf("age > hard TTL: %+v warned=%d, want fail-closed, no warning", res, warned)
	}
	res, warned = run(FailOpenSoftTTL)
	if res.Status != StatusFailOpen || warned != 0 {
		t.Errorf("age == soft TTL: %+v warned=%d, want silent fail-open", res, warned)
	}
}

func TestDefaultCheckIntervalIsSixHours(t *testing.T) {
	if DefaultCheckInterval != 6*time.Hour {
		t.Errorf("DefaultCheckInterval = %v, want 6h", DefaultCheckInterval)
	}
}
