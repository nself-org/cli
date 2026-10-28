package verify

// Purpose: trust on first use for the trust origin (D-1216). After the first
//          release that verifies, the project's effective origin URL is written
//          to ~/.nself/ci/trust/origin.url, so every later verification, and the
//          committed-inventory lookup, holds to that origin.
// Inputs:  the Git view, the operator home.
// Outputs: the pinned URL, or why nothing was pinned.
// Constraints: never replaces an existing pin file (hard link from a temp file,
//              which fails when the target exists). A URL that carries a
//              password or token is not written: the file is plain text and the
//              URL is printed. Only the operator home is written.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PinOriginOnFirstUse pins the project's origin when the operator has no pin.
// pinned is false, with a nil error, when there is nothing to do (a pin exists,
// or the project has no origin). A non-nil error means the origin was left
// unpinned and says why; it never means the release failed to verify.
func PinOriginOnFirstUse(ctx context.Context, g Git, home string) (url string, pinned bool, err error) {
	if g == nil || !filepath.IsAbs(home) {
		return "", false, nil
	}
	if _, has, err := loadOriginPin(home); has || err != nil {
		return "", false, err
	}
	ok, err := g.HasOrigin(ctx)
	if err != nil || !ok {
		return "", false, err
	}
	got, err := g.OriginURL(ctx)
	if err != nil {
		return "", false, fmt.Errorf("cannot read the origin URL to pin it: %w", err)
	}
	got = strings.TrimSpace(got)
	if !pinnableURL(got) {
		return "", false, errors.New("the origin URL is empty or carries credentials, so it was not written to " + originPinName + "; put the URL without credentials there to pin it")
	}
	dir := filepath.Join(home, ".nself", "ci", "trust")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, err
	}
	tmp, err := os.CreateTemp(dir, ".origin-*")
	if err != nil {
		return "", false, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, werr := tmp.WriteString("# pinned by nself after the first verified release; edit or delete this file to change it\n" + got + "\n")
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return "", false, werr
	}
	if err := os.Link(tmp.Name(), filepath.Join(dir, originPinName)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", false, nil // pinned by someone else meanwhile: theirs stands
		}
		return "", false, err
	}
	return got, true, nil
}

// pinnableURL reports whether u is a single-line URL with no userinfo secret.
// scp-like and ssh:// forms may name a user (git@host) but never a password.
func pinnableURL(u string) bool {
	if u == "" || strings.ContainsFunc(u, func(r rune) bool { return r < ' ' || r == 0x7f || r == ' ' }) {
		return false
	}
	authority := u
	if _, rest, ok := strings.Cut(u, "://"); ok {
		scheme, _, _ := strings.Cut(u, "://")
		authority, _, _ = strings.Cut(rest, "/")
		if i := strings.LastIndex(authority, "@"); i >= 0 && scheme != "ssh" {
			return false // https://token@host: the userinfo is a credential
		}
	} else {
		authority, _, _ = strings.Cut(u, ":")
	}
	if i := strings.LastIndex(authority, "@"); i >= 0 && strings.Contains(authority[:i], ":") {
		return false
	}
	return true
}
