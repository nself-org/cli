package simharness

// Purpose: generate the per-fleet ED25519 key pair, in the formats ssh and sshd expect.
// Inputs: a TB (only to place the private key file under its TempDir).
// Outputs: an authorized_keys line and the path of an OpenSSH-format private key (0600).
// Constraints: standard library only; the key never leaves the test's TempDir.

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"os"
	"path/filepath"
)

const keyType = "ssh-ed25519"

// newKeyPair writes a fresh private key to dir (0600) and returns the
// authorized_keys line for its public half plus the private key path.
func newKeyPair(dir string) (authorizedKey, keyPath string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	keyPath = filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyPath, marshalOpenSSHPrivate(pub, priv), 0o600); err != nil {
		return "", "", err
	}
	return keyType + " " + base64.StdEncoding.EncodeToString(pubBlob(pub)) + " simharness", keyPath, nil
}

// sshString appends a length-prefixed string (RFC 4251).
func sshString(b []byte, s []byte) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(len(s)))
	return append(b, s...)
}

// pubBlob is the SSH wire encoding of an ed25519 public key.
func pubBlob(pub ed25519.PublicKey) []byte {
	return sshString(sshString(nil, []byte(keyType)), pub)
}

// marshalOpenSSHPrivate encodes the key as an unencrypted openssh-key-v1 PEM
// ("OPENSSH PRIVATE KEY"), the format every OpenSSH client reads.
func marshalOpenSSHPrivate(pub ed25519.PublicKey, priv ed25519.PrivateKey) []byte {
	var check [4]byte
	_, _ = rand.Read(check[:])
	var priv1 []byte
	priv1 = append(priv1, check[:]...)
	priv1 = append(priv1, check[:]...)
	priv1 = sshString(priv1, []byte(keyType))
	priv1 = sshString(priv1, pub)
	priv1 = sshString(priv1, priv) // 64 bytes: seed || public
	priv1 = sshString(priv1, []byte("simharness"))
	for i := byte(1); len(priv1)%8 != 0; i++ {
		priv1 = append(priv1, i)
	}

	out := []byte("openssh-key-v1\x00")
	out = sshString(out, []byte("none")) // cipher
	out = sshString(out, []byte("none")) // kdf
	out = sshString(out, nil)            // kdf options
	out = binary.BigEndian.AppendUint32(out, 1)
	out = sshString(out, pubBlob(pub))
	out = sshString(out, priv1)
	return pem.EncodeToMemory(&pem.Block{Type: "OPENSSH PRIVATE KEY", Bytes: out})
}
