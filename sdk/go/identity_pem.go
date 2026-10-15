package sdk

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// parseIdentityPEM decodes an Identity from the PEM data produced by
// marshalIdentityPEM. Inputs: plugin name and PEM bytes. Outputs: identity.
// Constraints: requires a private key; derives a missing public key.
func parseIdentityPEM(pluginName string, data []byte) (*Identity, error) {
	var privKey ed25519.PrivateKey
	var pubKey ed25519.PublicKey

	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		switch block.Type {
		case pemTypePrivateKey:
			key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("identity: parse private key: %w", err)
			}
			k, ok := key.(ed25519.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("identity: key file contains %T, want ed25519.PrivateKey", key)
			}
			privKey = k
		case pemTypePublicKey:
			key, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("identity: parse public key: %w", err)
			}
			k, ok := key.(ed25519.PublicKey)
			if !ok {
				return nil, fmt.Errorf("identity: public key is %T, want ed25519.PublicKey", key)
			}
			pubKey = k
		}
	}

	if privKey == nil {
		return nil, fmt.Errorf("identity: no PRIVATE KEY block found in %s", pluginName)
	}
	if pubKey == nil {
		// Derive public key from private key if the PUBLIC KEY block was absent.
		pub, ok := privKey.Public().(ed25519.PublicKey)
		if !ok {
			return nil, fmt.Errorf("identity: could not derive public key")
		}
		pubKey = pub
	}

	return &Identity{
		PluginName: pluginName,
		PublicKey:  pubKey,
		privateKey: privKey,
	}, nil
}
