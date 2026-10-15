package plugin

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

// canonicalManifestFields holds exactly the 6 fields included in the signed
// canonical plugin object. Fields are declared alphabetically so encoding/json
// marshals them in that order (Go marshals struct fields in declaration order).
//
// This type is used ONLY for signing/verification — it is separate from
// PluginManifest to avoid coupling the signing contract to the full manifest schema.
type canonicalManifestFields struct {
	Bundle     string `json:"bundle"`
	Name       string `json:"name"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	Visibility string `json:"visibility"`
	Version    string `json:"version"`
}

// canonicalPluginObject returns the deterministic JSON serialization used as the
// signed message for plugin manifest Ed25519 signatures.
//
// Format: sorted-key JSON (alphabetical), no whitespace, UTF-8, no trailing newline.
// Fields: bundle, name, sha256, size, visibility, version.
//
// This function is the Go-side mirror of canonicalPluginObject() in plugin-sign.ts.
// The two implementations MUST produce byte-identical output for the same inputs.
// See golden fixture: cli/internal/plugin/testdata/canonical_golden.json
func canonicalPluginObject(bundle, name, sha256, visibility, version string, size int64) ([]byte, error) {
	if bundle == "" {
		return nil, fmt.Errorf("canonicalPluginObject: bundle must not be empty")
	}
	if name == "" {
		return nil, fmt.Errorf("canonicalPluginObject: name must not be empty")
	}
	if sha256 == "" {
		return nil, fmt.Errorf("canonicalPluginObject: sha256 must not be empty")
	}
	if version == "" {
		return nil, fmt.Errorf("canonicalPluginObject: version must not be empty")
	}
	if visibility == "" {
		return nil, fmt.Errorf("canonicalPluginObject: visibility must not be empty")
	}
	if size < 0 {
		return nil, fmt.Errorf("canonicalPluginObject: size must not be negative")
	}

	return json.Marshal(canonicalManifestFields{
		Bundle:     bundle,
		Name:       name,
		SHA256:     sha256,
		Size:       size,
		Visibility: visibility,
		Version:    version,
	})
}

// VerifyCanonicalSignature checks an Ed25519 signature over the canonical plugin
// object constructed from the provided fields.
//
// publicKeyHex is a hex-encoded 32-byte Ed25519 public key.
// signatureHex is a hex-encoded 64-byte Ed25519 signature.
//
// Returns nil on success, an error describing the failure otherwise.
func VerifyCanonicalSignature(bundle, name, sha256, visibility, version string, size int64, publicKeyHex, signatureHex string) error {
	canonical, err := canonicalPluginObject(bundle, name, sha256, visibility, version, size)
	if err != nil {
		return fmt.Errorf("failed to build canonical object: %w", err)
	}

	pubKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return fmt.Errorf("invalid public key hex: %w", err)
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key length: got %d bytes, want %d", len(pubKeyBytes), ed25519.PublicKeySize)
	}

	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length: got %d bytes, want %d", len(sigBytes), ed25519.SignatureSize)
	}

	if signing.VerifyEd25519(ed25519.PublicKey(pubKeyBytes), canonical, sigBytes) != nil {
		return fmt.Errorf("signature verification failed: canonical object or key mismatch")
	}
	return nil
}

// SignCanonicalObject signs the canonical plugin object with the provided Ed25519
// private key and returns the hex-encoded signature.
//
// privateKeyHex is a hex-encoded 64-byte Ed25519 private key (seed || public key).
// This function is intended for use in the ping_api worker and in tests; the CLI
// itself only verifies — it does not sign.
func SignCanonicalObject(bundle, name, sha256, visibility, version string, size int64, privateKeyHex string) (string, error) {
	canonical, err := canonicalPluginObject(bundle, name, sha256, visibility, version, size)
	if err != nil {
		return "", fmt.Errorf("failed to build canonical object: %w", err)
	}

	privKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("invalid private key hex: %w", err)
	}
	if len(privKeyBytes) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("invalid private key length: got %d bytes, want %d", len(privKeyBytes), ed25519.PrivateKeySize)
	}

	sig := ed25519.Sign(ed25519.PrivateKey(privKeyBytes), canonical)
	return hex.EncodeToString(sig), nil
}
