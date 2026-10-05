package signing

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"
)

// Signature is a key id and a raw Ed25519 signature.
type Signature struct {
	KeyID string
	Sig   []byte
}

// EncodeSig returns the canonical text form: base64 standard with padding.
func EncodeSig(sig []byte) string { return base64.StdEncoding.EncodeToString(sig) }

// DecodeSig strictly decodes a canonical signature: base64 standard with
// padding, no whitespace or newlines, zero trailing bits, 64 bytes.
func DecodeSig(b64 string) ([]byte, error) {
	raw, err := decodeStrict(b64)
	if err != nil {
		return nil, fmt.Errorf("%w: signature: %v", ErrMalformed, err)
	}
	if len(raw) != ed25519.SignatureSize {
		return nil, fmt.Errorf("%w: signature length %d", ErrMalformed, len(raw))
	}
	return raw, nil
}

// decodeStrict is base64 standard, padded, strict trailing bits, and no
// CR/LF (the stdlib decoder silently skips them).
func decodeStrict(s string) ([]byte, error) {
	if strings.ContainsAny(s, "\r\n") {
		return nil, fmt.Errorf("contains a line break")
	}
	return base64.StdEncoding.Strict().DecodeString(s)
}

// Signer produces raw Ed25519 signatures under one key id.
type Signer interface {
	KeyID() string
	Sign(msg []byte) ([]byte, error)
}

type ed25519Signer struct {
	id   string
	priv ed25519.PrivateKey
}

// NewEd25519Signer returns a Signer over priv. A bad id or key length is
// reported by Sign, never by a panic.
func NewEd25519Signer(id string, priv ed25519.PrivateKey) Signer {
	return &ed25519Signer{id: id, priv: priv}
}

func (s *ed25519Signer) KeyID() string { return s.id }

func (s *ed25519Signer) Sign(msg []byte) ([]byte, error) {
	if !validKeyID(s.id) {
		return nil, fmt.Errorf("%w: signer key id %q", ErrMalformed, clip(s.id))
	}
	if len(s.priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: signer %q private key length %d", ErrMalformed, s.id, len(s.priv))
	}
	return ed25519.Sign(s.priv, msg), nil
}

// ParsePKCS8PEM parses one "PRIVATE KEY" PEM block holding an Ed25519 key, as
// `openssl genpkey -algorithm ed25519` writes it. Trailing non-blank data, a
// second block, another block type or another algorithm is ErrMalformed. The
// error never carries key bytes.
func ParsePKCS8PEM(data []byte) (ed25519.PrivateKey, error) {
	block, rest := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%w: no PEM block", ErrMalformed)
	}
	if strings.TrimSpace(string(rest)) != "" {
		return nil, fmt.Errorf("%w: data after the PEM block", ErrMalformed)
	}
	if block.Type != "PRIVATE KEY" || len(block.Headers) != 0 {
		return nil, fmt.Errorf("%w: PEM block is not a bare PRIVATE KEY", ErrMalformed)
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: PKCS#8 parse failed", ErrMalformed)
	}
	priv, ok := k.(ed25519.PrivateKey)
	if !ok || len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: key is not Ed25519", ErrMalformed)
	}
	return priv, nil
}
