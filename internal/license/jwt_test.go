package license

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// useTestPingKey swaps PingKeys for one test key (kid "t1") and restores it.
func useTestPingKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	orig := PingKeys
	PingKeys = []PingKey{{ID: "t1", Public: pub}}
	t.Cleanup(func() { PingKeys = orig })
	return priv
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func jsonB64(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b64(b)
}

func goodClaims() map[string]any {
	return map[string]any{
		"sub": "abc123", "tier": "plus", "plugins": []string{"ai", "claw"},
		"iat": 1700000000, "exp": 1700086400, "iss": "ping.nself.org", "aud": "nself-cli",
		"revocable": true, "renewal": "2027-01-01T00:00:00.000Z",
	}
}

// signEdDSA builds a token shaped like ping_api's issueLicenseJWT output.
func signEdDSA(t *testing.T, priv ed25519.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	in := jsonB64(t, header) + "." + jsonB64(t, claims)
	return in + "." + b64(ed25519.Sign(priv, []byte(in)))
}

func goodHeader() map[string]any { return map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "t1"} }

func TestParseLicenseJWT(t *testing.T) {
	priv := useTestPingKey(t)
	other := func() ed25519.PrivateKey { _, p, _ := ed25519.GenerateKey(rand.Reader); return p }()

	valid := signEdDSA(t, priv, goodHeader(), goodClaims())
	vp := strings.Split(valid, ".")

	hs := func() string { // HS256 token "signed" with the public key bytes (key confusion attempt)
		in := jsonB64(t, map[string]any{"alg": "HS256", "typ": "JWT", "kid": "t1"}) + "." + jsonB64(t, goodClaims())
		m := hmac.New(sha256.New, PingKeys[0].Public)
		m.Write([]byte(in))
		return in + "." + b64(m.Sum(nil))
	}()
	noneTok := jsonB64(t, map[string]any{"alg": "none", "typ": "JWT", "kid": "t1"}) + "." + jsonB64(t, goodClaims()) + "."
	noneSigned := signEdDSA(t, priv, map[string]any{"alg": "none", "kid": "t1"}, goodClaims())
	wrongIss := goodClaims()
	wrongIss["iss"] = "evil.example"
	wrongAud := goodClaims()
	wrongAud["aud"] = "someone-else"
	audList := goodClaims()
	audList["aud"] = []string{"x", "nself-cli"}
	tampered := vp[0] + "." + jsonB64(t, func() map[string]any { c := goodClaims(); c["tier"] = "owner"; return c }()) + "." + vp[2]
	tamperedHdr := jsonB64(t, map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "t1", "x": 1}) + "." + vp[1] + "." + vp[2]
	crit := map[string]any{"alg": "EdDSA", "kid": "t1", "crit": []string{"exp"}}

	cases := []struct {
		name  string
		token string
		ok    bool
	}{
		{"valid", valid, true},
		{"valid with aud list", signEdDSA(t, priv, goodHeader(), audList), true},
		{"alg none unsigned", noneTok, false},
		{"alg none with a real signature", noneSigned, false},
		{"alg HS256", hs, false},
		{"unknown kid", signEdDSA(t, priv, map[string]any{"alg": "EdDSA", "kid": "zz"}, goodClaims()), false},
		{"empty kid", signEdDSA(t, priv, map[string]any{"alg": "EdDSA"}, goodClaims()), false},
		{"signed by another key", signEdDSA(t, other, goodHeader(), goodClaims()), false},
		{"tampered payload", tampered, false},
		{"tampered header", tamperedHdr, false},
		{"wrong issuer", signEdDSA(t, priv, goodHeader(), wrongIss), false},
		{"wrong audience", signEdDSA(t, priv, goodHeader(), wrongAud), false},
		{"crit header", signEdDSA(t, priv, crit, goodClaims()), false},
		{"two segments", vp[0] + "." + vp[1], false},
		{"four segments", valid + ".x", false},
		{"empty signature", vp[0] + "." + vp[1] + ".", false},
		{"short signature", vp[0] + "." + vp[1] + "." + b64([]byte("short")), false},
		{"not base64", "!!!.@@@.###", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseLicenseJWT(c.token)
			if c.ok {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got.Sub != "abc123" || got.Tier != "plus" || len(got.Plugins) != 2 || got.Iat != 1700000000 || got.Exp != 1700086400 || !got.Revocable {
					t.Errorf("claims not decoded: %+v", got)
				}
				return
			}
			if got != nil || !errors.Is(err, ErrInvalidLicenseJWT) {
				t.Fatalf("want ErrInvalidLicenseJWT and no claims, got (%v, %v)", got, err)
			}
		})
	}
}

func TestParseLicenseJWT_PicksKeyByKid(t *testing.T) {
	priv := useTestPingKey(t)
	_, priv2, _ := ed25519.GenerateKey(rand.Reader)
	PingKeys = append(PingKeys, PingKey{ID: "t2", Public: priv2.Public().(ed25519.PublicKey)})
	h2 := goodHeader()
	h2["kid"] = "t2"
	if _, err := ParseLicenseJWT(signEdDSA(t, priv2, h2, goodClaims())); err != nil {
		t.Errorf("kid t2 signed by key t2 should verify: %v", err)
	}
	// A token that names kid t1 but is signed by key t2 must fail: the kid selects the key.
	if _, err := ParseLicenseJWT(signEdDSA(t, priv2, goodHeader(), goodClaims())); err == nil {
		t.Error("kid t1 signed by key t2 verified")
	}
	if _, err := ParseLicenseJWT(signEdDSA(t, priv, goodHeader(), goodClaims())); err != nil {
		t.Errorf("kid t1 signed by key t1: %v", err)
	}
}

func TestLicenseClaims_Expired(t *testing.T) {
	c := &LicenseClaims{Exp: 100}
	if c.Expired(99) || !c.Expired(100) || !c.Expired(101) {
		t.Error("Expired boundary: exp itself counts as expired")
	}
}

// TestValidateResponseDecodesPluginsAllowed decodes a body in the shape
// routes/license-validate.ts sends (plugins_allowed plus the jwt trio, and no
// plugins/signature/key_id) and checks the embedded JWT verifies.
func TestValidateResponseDecodesPluginsAllowed(t *testing.T) {
	priv := useTestPingKey(t)
	tok := signEdDSA(t, priv, goodHeader(), goodClaims())
	body, _ := json.Marshal(map[string]any{
		"valid": true, "tier": "plus", "product": "plus",
		"products_covered": []string{"claw"}, "plugins_allowed": []string{"ai", "claw", "voice"},
		"features": []string{}, "can_install_plugin": true, "upgrade_required": false,
		"expires_at": "2027-01-01T00:00:00.000Z", "key_type": "product",
		"jwt": tok, "jwt_kid": "t1", "jwt_expires_at": 1700086400,
	})
	var vr ValidateResponse
	if err := json.Unmarshal(body, &vr); err != nil {
		t.Fatal(err)
	}
	if got := vr.AllowedPlugins(); len(got) != 3 || got[2] != "voice" {
		t.Errorf("AllowedPlugins = %v, want the plugins_allowed list", got)
	}
	if len(vr.Plugins) != 0 {
		t.Errorf("legacy Plugins should stay empty for a real response, got %v", vr.Plugins)
	}
	if vr.JWT != tok || vr.JWTKid != "t1" || vr.JWTExpiresAt != 1700086400 {
		t.Errorf("jwt fields not decoded: %q %q %d", vr.JWT, vr.JWTKid, vr.JWTExpiresAt)
	}
	if _, err := ParseLicenseJWT(vr.JWT); err != nil {
		t.Errorf("decoded jwt should verify: %v", err)
	}
	// Absent plugins_allowed falls back to the legacy field; an empty list does not.
	legacy := ValidateResponse{Plugins: []string{"old"}}
	if got := legacy.AllowedPlugins(); len(got) != 1 || got[0] != "old" {
		t.Errorf("fallback = %v", got)
	}
	empty := ValidateResponse{PluginsAllowed: []string{}, Plugins: []string{"old"}}
	if got := empty.AllowedPlugins(); len(got) != 0 {
		t.Errorf("an explicit empty plugins_allowed must win over legacy plugins, got %v", got)
	}
}
