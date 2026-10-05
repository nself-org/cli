package simharness

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestKeyPairFormatAndMode(t *testing.T) {
	auth, path, err := newKeyPair(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("key file %v %v, want 0600", st, err)
	}
	raw, _ := os.ReadFile(path)
	blk, _ := pem.Decode(raw)
	if blk == nil || blk.Type != "OPENSSH PRIVATE KEY" {
		t.Fatalf("not an OpenSSH PEM: %q", raw)
	}
	b := blk.Bytes
	magic := "openssh-key-v1\x00"
	if !bytes.HasPrefix(b, []byte(magic)) {
		t.Fatal("missing openssh-key-v1 magic")
	}
	b = b[len(magic):]
	read := func() []byte {
		n := binary.BigEndian.Uint32(b)
		out := b[4 : 4+n]
		b = b[4+n:]
		return out
	}
	if string(read()) != "none" || string(read()) != "none" || len(read()) != 0 {
		t.Fatal("cipher, kdf or kdf options not empty")
	}
	if binary.BigEndian.Uint32(b) != 1 {
		t.Fatal("key count != 1")
	}
	b = b[4:]
	pubBlobBytes := read()
	priv := read()
	if len(priv)%8 != 0 {
		t.Fatalf("private block length %d is not a multiple of 8", len(priv))
	}
	if !bytes.Equal(priv[:4], priv[4:8]) {
		t.Fatal("check ints differ")
	}
	f := strings.Fields(auth)
	if len(f) != 3 || f[0] != "ssh-ed25519" {
		t.Fatalf("authorized key = %q", auth)
	}
	wire, err := base64.StdEncoding.DecodeString(f[1])
	if err != nil || !bytes.Equal(wire, pubBlobBytes) {
		t.Fatalf("authorized key does not match the private key's public blob")
	}
	// the 64-byte private value must contain its own public half
	p := priv[8:]
	n := binary.BigEndian.Uint32(p)
	p = p[4+n:] // key type
	n = binary.BigEndian.Uint32(p)
	pub := p[4 : 4+n]
	p = p[4+n:]
	n = binary.BigEndian.Uint32(p)
	sk := p[4 : 4+n]
	if len(sk) != ed25519.PrivateKeySize || !bytes.Equal(sk[32:], pub) {
		t.Fatal("private value is not seed||public")
	}
}

// OpenSSH itself must read the key and derive the same public key.
func TestKeyPairReadByOpenSSH(t *testing.T) {
	kg, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not installed")
	}
	auth, path, err := newKeyPair(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(kg, "-y", "-f", path).Output()
	if err != nil {
		t.Fatalf("ssh-keygen -y: %v", err)
	}
	got := strings.Fields(string(out))
	want := strings.Fields(auth)
	if len(got) < 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ssh-keygen derived %q, want %q", got, want)
	}
}

func TestKeyPairsDiffer(t *testing.T) {
	a, _, _ := newKeyPair(t.TempDir())
	b, _, _ := newKeyPair(t.TempDir())
	if a == b {
		t.Fatal("two key pairs are identical")
	}
}
