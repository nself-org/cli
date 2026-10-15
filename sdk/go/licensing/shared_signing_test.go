package licensing

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/licensing/licensetest"
)

func TestEntitlementVerificationRejectsSmallOrderKey(t *testing.T) {
	pub, sig := make(ed25519.PublicKey, 32), make([]byte, 64)
	pub[0], sig[0] = 1, 1
	licensetest.UseKey(t, pub)
	_, priv := syntheticKey(t)
	now := time.Now()
	parts := strings.Split(tokenFor(t, priv, header(), claimsAt(now)), ".")
	parts[2] = base64.RawURLEncoding.EncodeToString(sig)
	if err := verifyToken(strings.Join(parts, "."), "paid", now); err == nil {
		t.Fatal("entitlement accepted a universal forgery")
	}
}
