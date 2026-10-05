package signingtest_test

import (
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/cli/sdk/go/v2/signing/signingtest"
)

func TestNewKey(t *testing.T) {
	k1, s1 := signingtest.NewKey(t, signing.PurposeCINode)
	k2, _ := signingtest.NewKey(t, signing.PurposeCINode)
	if k1.ID == k2.ID {
		t.Fatal("keys are not fresh")
	}
	if k1.ID != signing.KeyID(signing.PurposeCINode, k1.Public) || k1.Purpose != signing.PurposeCINode || s1.KeyID() != k1.ID {
		t.Fatalf("bad key: %+v", k1)
	}
	v, err := signing.NewVerifier(signing.PurposeCINode, []signing.Key{k1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := s1.Sign([]byte("m"))
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Verify([]byte("m"), signing.Signature{KeyID: k1.ID, Sig: sig}); err != nil {
		t.Fatal(err)
	}
}
