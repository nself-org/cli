package sdk

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestIdentityVerificationRejectsSmallOrderKey(t *testing.T) {
	pub, sig := make(ed25519.PublicKey, 32), make([]byte, 64)
	pub[0], sig[0] = 1, 1
	id := &Identity{PublicKey: pub}
	if id.Verify([]byte("forged identity"), sig) {
		t.Error("identity accepted a universal forgery")
	}
	req := httptest.NewRequest("POST", "/peer", nil)
	req.Header.Set(TimestampHeader, strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set(SignatureHeader, base64.StdEncoding.EncodeToString(sig))
	if err := VerifyRequest(req, pub, nil); err == nil {
		t.Error("request accepted a universal forgery")
	}
}
