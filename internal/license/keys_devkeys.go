//go:build nself_devkeys

// Package license — keys_devkeys.go: extra verification keys for developer
// builds tagged nself_devkeys (never used by goreleaser, homebrew or any
// release script).
//
// Purpose: let developers run against a local ping stub signed by a test key.
// Inputs: -X ...license.licensePubKeyHex=<hex> and LICENSE_PUBLIC_KEY_OVERRIDE.
// Outputs: those keys appended to PingKeys by GetPublicKeys, with one stderr
// warning per process the first time one is used.
package license

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
)

// devKeysBuild reports whether this binary was built with the nself_devkeys tag.
const devKeysBuild = true

// licensePubKeyHex may be set with -X in a dev build.
var licensePubKeyHex = ""

var devKeyWarn sync.Once

// resetDevKeyWarning re-arms the one-time warning (tests).
func resetDevKeyWarning() { devKeyWarn = sync.Once{} }

// extraKeys returns the ldflags key and the override key when they decode to a
// usable Ed25519 key.
func extraKeys() []PublicKeyEntry {
	var out []PublicKeyEntry
	for _, src := range []struct{ name, val string }{
		{"ldflags licensePubKeyHex", licensePubKeyHex},
		{"LICENSE_PUBLIC_KEY_OVERRIDE", os.Getenv("LICENSE_PUBLIC_KEY_OVERRIDE")},
	} {
		if src.val == "" {
			continue
		}
		b, err := hex.DecodeString(src.val)
		if err != nil || !usableKey(ed25519.PublicKey(b)) {
			continue
		}
		devKeyWarn.Do(func() {
			fmt.Fprintf(os.Stderr, "warning: nself_devkeys build: trusting a developer licence key from %s\n", src.name)
		})
		out = append(out, PublicKeyEntry{ID: 1, KID: "dev", Key: ed25519.PublicKey(b)})
	}
	return out
}
