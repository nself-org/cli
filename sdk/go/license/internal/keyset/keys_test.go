package keyset

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"reflect"
	"testing"
)

func TestDecodeKeys(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if got := decode(hex.EncodeToString(pub)); !reflect.DeepEqual(got, pub) {
		t.Fatal("valid public key changed")
	}
	for _, value := range []string{"", "invalid", "0102"} {
		if decode(value) != nil {
			t.Fatalf("malformed root %q accepted", value)
		}
	}
}

func TestSnapshotAndRestore(t *testing.T) {
	before := Snapshot()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("temporary", func(t *testing.T) {
		UseTestingKey(t, pub)
		pub[0] ^= 255
		if reflect.DeepEqual(Snapshot()[0].Public, pub) {
			t.Fatal("input aliases active roots")
		}
		snapshot := Snapshot()
		snapshot[0].Public[0] ^= 255
		snapshot[0].ID = "unknown"
		if reflect.DeepEqual(Snapshot(), snapshot) {
			t.Fatal("snapshot aliases roots")
		}
	})
	if !reflect.DeepEqual(Snapshot(), before) {
		t.Fatal("testing roots not restored")
	}
}
