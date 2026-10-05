package signing_test

import (
	"errors"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/cli/sdk/go/v2/signing/signingtest"
)

var allSentinels = []error{
	signing.ErrUnknownKey, signing.ErrRevoked, signing.ErrWrongPurpose, signing.ErrWrongScope,
	signing.ErrBadSignature, signing.ErrMalformed, signing.ErrExpired, signing.ErrNotYetValid,
}

var allPurposes = []signing.Purpose{
	signing.PurposePlugins, signing.PurposeAgent, signing.PurposeCIRelease,
	signing.PurposeCINode, signing.PurposeCIAudit,
}

// only asserts err matches want and no other sentinel.
func only(t *testing.T, err, want error) {
	t.Helper()
	if want == nil {
		if err != nil {
			t.Fatalf("want nil, got %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("want %v, got nil", want)
	}
	for _, s := range allSentinels {
		if got := errors.Is(err, s); got != (s == want) {
			t.Fatalf("errors.Is(%v, %v) = %v, want %v", err, s, got, s == want)
		}
	}
}

func sign(t *testing.T, s signing.Signer, msg []byte) signing.Signature {
	t.Helper()
	b, err := s.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}
	return signing.Signature{KeyID: s.KeyID(), Sig: b}
}

func newVerifier(t *testing.T, p signing.Purpose, keys []signing.Key, revoked []string, opts ...signing.Option) *signing.Verifier {
	t.Helper()
	v, err := signing.NewVerifier(p, keys, revoked, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func newKeyPair(t *testing.T, p signing.Purpose) (signing.Key, signing.Signer) {
	t.Helper()
	return signingtest.NewKey(t, p)
}
