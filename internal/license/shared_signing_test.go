package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestLicenseVerificationRejectsSmallOrderKey(t *testing.T) {
	pub, sig := make(ed25519.PublicKey, 32), make([]byte, 64)
	pub[0], sig[0] = 1, 1
	installTestPubKey(t, pub)
	original := PingKeys
	PingKeys = []PingKey{{ID: "t1", Public: pub}}
	t.Cleanup(func() { PingKeys = original })
	if err := verifyResponseSig([]byte("forged response"), hex.EncodeToString(sig)); err == nil {
		t.Error("response accepted a universal forgery")
	}
	entry := CacheEntry{Signature: hex.EncodeToString(sig)}
	if entry.verifyLegacy() {
		t.Error("legacy cache accepted a universal forgery")
	}
	list := &RevocationList{Signature: base64.StdEncoding.EncodeToString(sig)}
	if VerifyRevocationSignature(list) {
		t.Error("revocation list accepted a universal forgery")
	}
	token := jsonB64(t, goodHeader()) + "." + jsonB64(t, goodClaims()) + "." + b64(sig)
	if _, err := ParseLicenseJWT(token); err == nil {
		t.Error("licence JWT accepted a universal forgery")
	}
	body, err := json.Marshal(ValidateResponse{Valid: true, Tier: "plus", JWT: token})
	if err != nil {
		t.Fatal(err)
	}
	entry = CacheEntry{KeyHash: "abc123", Tier: "plus", JWT: token,
		RawBody: string(body), BodySig: hex.EncodeToString(sig)}
	if entry.verifySignedBody() {
		t.Error("signed-body cache accepted a universal forgery")
	}
}
