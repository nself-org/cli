package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/license/licensetest"
)

func syntheticKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}
func tokenFor(t *testing.T, priv ed25519.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	enc := base64.RawURLEncoding
	h, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	c, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	input := enc.EncodeToString(h) + "." + enc.EncodeToString(c)
	return input + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(input)))
}
func claimsAt(now time.Time) map[string]any {
	return map[string]any{"iss": "ping.nself.org", "aud": "nself-cli", "plugins": []string{"paid"}, "exp": now.Unix()}
}
func header() map[string]any { return map[string]any{"alg": "EdDSA", "kid": "1"} }
func setToken(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "entitlement.jwt")
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(FileEnv, path)
	return path
}

func TestVerifyEntitlementValid(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	now := time.Now()
	for _, plugins := range [][]string{{"paid"}, {"*"}} {
		c := claimsAt(now)
		c["plugins"] = plugins
		setToken(t, tokenFor(t, priv, header(), c))
		if err := VerifyEntitlement("paid", now); err != nil {
			t.Fatal(err)
		}
	}
	c := claimsAt(now)
	c["aud"] = []string{"other", "nself-cli"}
	setToken(t, tokenFor(t, priv, header(), c))
	if err := VerifyEntitlement("paid", now); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyEntitlementRejects(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	now := time.Now()
	cases := map[string]func(map[string]any, map[string]any){
		"algorithm":       func(h, c map[string]any) { h["alg"] = "HS256" },
		"none":            func(h, c map[string]any) { h["alg"] = "none" },
		"kid":             func(h, c map[string]any) { h["kid"] = "unknown" },
		"missing kid":     func(h, c map[string]any) { delete(h, "kid") },
		"critical":        func(h, c map[string]any) { h["crit"] = []string{"unknown"} },
		"issuer":          func(h, c map[string]any) { c["iss"] = "evil" },
		"audience":        func(h, c map[string]any) { c["aud"] = "evil" },
		"audience type":   func(h, c map[string]any) { c["aud"] = 123 },
		"audience array":  func(h, c map[string]any) { c["aud"] = []string{"evil", "other"} },
		"plugin":          func(h, c map[string]any) { c["plugins"] = []string{"other"} },
		"missing expiry":  func(h, c map[string]any) { delete(c, "exp") },
		"overflow expiry": func(h, c map[string]any) { c["exp"] = int64(1<<63 - 1) },
		"expiry type":     func(h, c map[string]any) { c["exp"] = "tomorrow" },
	}
	for name, modify := range cases {
		t.Run(name, func(t *testing.T) {
			h, c := header(), claimsAt(now)
			modify(h, c)
			setToken(t, tokenFor(t, priv, h, c))
			if VerifyEntitlement("paid", now) == nil {
				t.Fatal("accepted invalid token")
			}
		})
	}
	_, other := syntheticKey(t)
	setToken(t, tokenFor(t, other, header(), claimsAt(now)))
	if VerifyEntitlement("paid", now) == nil {
		t.Fatal("accepted forged token")
	}
	for _, token := range []string{"", "a", "a.b.c", "..", tokenFor(t, priv, header(), claimsAt(now)) + ".extra"} {
		setToken(t, token)
		if VerifyEntitlement("paid", now) == nil {
			t.Fatal("accepted malformed token")
		}
	}
	setToken(t, tokenFor(t, priv, header(), claimsAt(now)))
	if VerifyEntitlement("", now) == nil {
		t.Fatal("accepted empty slug")
	}
}

func TestVerifyEntitlementGrace(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	exp := time.Unix(1800000000, 0)
	setToken(t, tokenFor(t, priv, header(), claimsAt(exp)))
	for _, now := range []time.Time{exp.Add(-time.Second), exp, exp.Add(DefaultGracePeriod - time.Second), exp.Add(DefaultGracePeriod)} {
		if VerifyEntitlement("paid", now) != nil {
			t.Fatalf("rejected within grace at %s", now)
		}
	}
	if VerifyEntitlement("paid", exp.Add(DefaultGracePeriod+time.Nanosecond)) == nil {
		t.Fatal("accepted beyond grace")
	}
}

func TestVerifyEntitlementEnvKeyFileIgnored(t *testing.T) {
	pub, priv := syntheticKey(t)
	path := filepath.Join(t.TempDir(), "fake-public-key")
	if err := os.WriteFile(path, []byte(hex.EncodeToString(pub)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", path)
	t.Setenv("NSELF_LICENSE_PUBKEY_HEX", hex.EncodeToString(pub))
	setToken(t, tokenFor(t, priv, header(), claimsAt(time.Now())))
	if VerifyEntitlement("paid", time.Now()) == nil {
		t.Fatal("accepted environment root")
	}
}

func TestKeyEnvIgnoredWithoutTag(t *testing.T) {
	if taggedTestKey != nil {
		t.Skip("default-tag contract")
	}
	pub, priv := syntheticKey(t)
	t.Setenv("NSELF_ENTITLEMENT_TEST_PUBKEY", hex.EncodeToString(pub))
	now := time.Now()
	setToken(t, tokenFor(t, priv, header(), claimsAt(now)))
	if VerifyEntitlement("paid", now) == nil {
		t.Fatal("test env affected release build")
	}
}

func TestKeyEnvWithTag(t *testing.T) {
	if taggedTestKey == nil {
		t.Skip("test-tag contract")
	}
	pub, priv := syntheticKey(t)
	t.Setenv("NSELF_ENTITLEMENT_TEST_PUBKEY", hex.EncodeToString(pub))
	now := time.Now()
	setToken(t, tokenFor(t, priv, header(), claimsAt(now)))
	if err := VerifyEntitlement("paid", now); err != nil {
		t.Fatal(err)
	}
	h := header()
	h["kid"] = "unknown"
	setToken(t, tokenFor(t, priv, h, claimsAt(now)))
	if VerifyEntitlement("paid", now) == nil {
		t.Fatal("test root accepted unknown kid")
	}
	t.Setenv("NSELF_ENTITLEMENT_TEST_PUBKEY", strings.Repeat("0", 64))
	setToken(t, tokenFor(t, priv, header(), claimsAt(now)))
	if VerifyEntitlement("paid", now) == nil {
		t.Fatal("accepted zero key")
	}
}

func TestVerifyEntitlementFileBoundary(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	now := time.Unix(1800000000, 0)
	token := tokenFor(t, priv, header(), claimsAt(now))
	const fileLimit = 1048576
	if maxTokenBytes != fileLimit {
		t.Fatal("file limit changed")
	}
	setToken(t, token+strings.Repeat(" ", fileLimit-len(token)))
	if err := VerifyEntitlement("paid", now); err != nil {
		t.Fatalf("valid maximum-size file rejected: %v", err)
	}
	setToken(t, token+strings.Repeat(" ", fileLimit-len(token)+1))
	if VerifyEntitlement("paid", now) == nil {
		t.Fatal("oversized signed token accepted")
	}
}

func TestVerifyEntitlementExpiryBoundaries(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	const graceSeconds = int64(2592000)
	if DefaultGracePeriod != time.Duration(graceSeconds)*time.Second {
		t.Fatal("grace must be exactly 30 days")
	}
	const upper = int64(1<<63-1) - graceSeconds
	for _, tc := range []struct {
		exp   int64
		valid bool
	}{{-1, false}, {0, false}, {1, true}, {upper, true}, {upper + 1, false}} {
		c := claimsAt(time.Unix(0, 0))
		c["exp"] = tc.exp
		setToken(t, tokenFor(t, priv, header(), c))
		if got := VerifyEntitlement("paid", time.Unix(0, 0)) == nil; got != tc.valid {
			t.Fatalf("exp=%d valid=%v want=%v", tc.exp, got, tc.valid)
		}
	}
}

func TestSigningKeyTaggedKid(t *testing.T) {
	pub, _ := syntheticKey(t)
	previous := taggedTestKey
	taggedTestKey = func() ed25519.PublicKey { return pub }
	t.Cleanup(func() { taggedTestKey = previous })
	if !reflect.DeepEqual(signingKey("1"), pub) {
		t.Fatal("tagged root not selected for known kid")
	}
	if signingKey("unknown") != nil {
		t.Fatal("tagged root accepted unknown kid")
	}
	if usableKey(nil) || usableKey(make(ed25519.PublicKey, 31)) {
		t.Fatal("malformed root accepted")
	}
}

func TestLicensetestUseKeyRestores(t *testing.T) {
	before := PingKeys()
	pub, _ := syntheticKey(t)
	t.Run("temporary", func(t *testing.T) {
		licensetest.UseKey(t, pub)
		if !reflect.DeepEqual(PingKeys()[0].Public, pub) {
			t.Fatal("key not installed")
		}
		snapshot := PingKeys()
		snapshot[0].Public[0] ^= 255
		if reflect.DeepEqual(PingKeys(), snapshot) {
			t.Fatal("snapshot mutates roots")
		}
	})
	if !reflect.DeepEqual(PingKeys(), before) {
		t.Fatal("roots not restored")
	}
	t.Run("zero root", func(t *testing.T) {
		licensetest.UseKey(t, make(ed25519.PublicKey, ed25519.PublicKeySize))
		if signingKey("1") != nil {
			t.Fatal("accepted zero root")
		}
	})
}
