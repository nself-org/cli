package signature_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
	"github.com/nself-org/cli/sdk/go/v2/signing"
)

const (
	slug    = "ci"
	version = "1.2.7"
)

// digest returns the lowercase hex sha256 of s.
func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// code returns the E-code of err, or "" for nil or an uncoded error.
func code(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

// fixture installs one key per tier and returns the signers.
type fixture struct {
	free, licensed         signature.Key
	freeSign, licensedSign signing.Signer
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	var f fixture
	f.free, f.freeSign = signaturetest.NewKey(t, signature.TierFree)
	f.licensed, f.licensedSign = signaturetest.NewKey(t, signature.TierLicensed)
	signaturetest.UseKeys(t, []signature.Key{f.free, f.licensed}, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{})
	return f
}
