// Command signer signs every entry of a plugin catalogue with a freshly generated
// ed25519 key (P7-CANON-13, scripts/ci/ports-fixture-registry.sh).
//
// Purpose: give the ports fixture registry release signatures the CLI accepts once the
// test pins the matching public key; the key exists only in this process.
// Usage:   signer <registry.json> <key.pub>
// Inputs:  a catalogue {"plugins":[{name, version, checksum, ...}]}.
// Outputs: registry.json rewritten with a release_signature on every entry; key.pub
//
//	holds the standard base64 of the raw 32-byte public key. The private key is never written.
//
// Exit:    0 ok, 1 failure, 2 usage.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/sdk/go/v2/signing"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: signer <registry.json> <key.pub>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "signer:", err)
		os.Exit(1)
	}
}

func run(registryPath, pubPath string) error {
	data, err := os.ReadFile(registryPath)
	if err != nil {
		return err
	}
	var doc struct {
		Plugins []map[string]any `json:"plugins"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%s: %w", registryPath, err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	signer := signing.NewEd25519Signer(signing.PurposePlugins, priv)
	for _, e := range doc.Plugins {
		name, _ := e["name"].(string)
		version, _ := e["version"].(string)
		sum, _ := e["checksum"].(string)
		if name == "" || version == "" || sum == "" {
			return fmt.Errorf("an entry lacks name, version or checksum: %v", e)
		}
		sig, err := signer.Sign(signature.SigningInput(name, version, sum))
		if err != nil {
			return err
		}
		e["release_signature"] = map[string]string{"key_id": signer.KeyID(), "alg": signature.Alg, "sig": signing.EncodeSig(sig)}
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(registryPath, append(out, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(pubPath, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644)
}
