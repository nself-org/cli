package acme

// image.go — the ACME client image reference.
//
// Purpose: LegoImage is the one-shot goacme/lego image the ACME runner starts.
// Inputs: the embedded image lock (internal/compose/images.yaml entry "lego").
// Outputs: LegoImage, always the digest-pinned lock reference, in both pinning
// modes: this image runs with certificate credentials, so it is never the
// unpinned legacy spelling.
// Constraints: no literal image string here (the image literal scan test).

import "github.com/nself-org/cli/internal/compose"

// LegoImage is the pinned ACME client image: goacme/lego at its multi-arch
// index digest, read from the image lock.
var LegoImage = lockedLego()

func lockedLego() string {
	r, ok := compose.LockedRef("lego")
	if !ok {
		return ""
	}
	return r.String()
}
