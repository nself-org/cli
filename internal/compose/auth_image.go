package compose

// auth_image.go — the auth service image per compat mode (EPIC D17, P7-LIVE-20).
//
// Purpose: nhost/hasura-auth was renamed nhost/auth (0.36.0 has the same index
// digest under both names). The lock carries auth-v14 (nhost/auth:0.36.0) and
// auth (the current release). The current release refuses to start without
// AUTH_ENCRYPTION_KEY and encrypts every TOTP secret with it on first start, so
// it is rendered only when the env sets the key (lead ruling, LIVE
// Invalidations 2026-10-11).
// Inputs: the generator config (AUTH_VERSION, the AUTH_* passthrough), the image
// lock and the project lock pins.
// Outputs: the auth image reference, or an error when it is the current release
// and the key is missing or malformed.
// Constraints: one compat.V15 gate (AuthEntry); the key is validated as it is
// forwarded (no trimming) and never printed.

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/nself-org/cli/internal/compat"
)

// AuthEncryptionKeyEnv is the key nhost/auth 0.43.0 and later require: 32 bytes, hex.
const AuthEncryptionKeyEnv = "AUTH_ENCRYPTION_KEY"

// frozenAuthRepo is the pre-rename repository, frozen at 0.42.x upstream.
const frozenAuthRepo = "nhost/hasura-auth"

var (
	authKeyRE = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	// authVersionRE reads an AUTH_VERSION that names a release (0.45.0, v0.52.1-beta1).
	authVersionRE = regexp.MustCompile(`^v?(\d+)\.(\d+)\.\d+(-[0-9A-Za-z.-]+)?$`)
)

// AuthRefNeedsKey reports whether an auth image reference is a release from
// 0.43.0 (tag) or the current lock entry (digest-only), so it needs the key and
// has migrated a database it ran on one way. The update plan asks it (D-0661).
func AuthRefNeedsKey(ref string) bool {
	cur, _ := LockedRef("auth")
	return authTagNeedsKey(authTag(ref)) || (cur.IndexDigest != "" && strings.HasSuffix(ref, "@"+cur.IndexDigest))
}

// authTag is the tag of an image reference (digest dropped), "" when it has none.
func authTag(ref string) string {
	base := imageBase(ref)
	if i := strings.LastIndex(base, ":"); i > strings.LastIndex(base, "/") {
		return base[i+1:]
	}
	return ""
}

// authTagNeedsKey reports whether an explicit release tag is 0.43.0 or later.
func authTagNeedsKey(tag string) bool {
	m := authVersionRE.FindStringSubmatch(tag)
	return m != nil && (m[1] != "0" || len(m[2]) > 2 || (len(m[2]) == 2 && m[2] >= "43"))
}

// AuthKeySet reports whether passthrough (config.Passthrough, forwarded to the
// container verbatim) holds a valid AUTH_ENCRYPTION_KEY.
func AuthKeySet(passthrough map[string]string) bool {
	return authKeyRE.MatchString(passthrough[AuthEncryptionKeyEnv])
}

// AuthEntry names the image lock entry the auth service runs by default:
// auth-v14 (nhost/auth:0.36.0) in v1.4; in v1.5 auth (the current release) once
// the env sets a valid AUTH_ENCRYPTION_KEY, else auth-v14 with keyMissing true.
// The update plan asks it too (D-0661).
func AuthEntry(passthrough map[string]string) (entry string, keyMissing bool) {
	// compat.V15(P7-LIVE-20): auth runs nhost/auth:0.36.0 (auth-v14) -> the current nhost/auth release (auth) when AUTH_ENCRYPTION_KEY is set
	if !compat.V15() {
		return "auth-v14", false
	}
	if AuthKeySet(passthrough) {
		return "auth", false
	}
	return "auth-v14", true
}

// authImage renders the auth image. AUTH_VERSION blank or 0.36.0 (the config
// default nself init writes into .env) renders the AuthEntry entry; the current
// release's version renders that entry in any mode when the key is set (else
// auth-v14 and the notice). Neither is an explicit override
// (Ref.IsDefaultVersion), so a project lock pin wins over both; a pin in the
// pre-rename nhost/hasura-auth spelling is returned as is (same digest;
// ResolveImage would drop it as another repository). Any other value is an
// explicit tag and outranks the pin: a release from 0.43.0 on nhost/auth, any
// other tag on the frozen nhost/hasura-auth line it always named. The current release without a
// valid key, rendered or pinned at any release from 0.43.0 (for example a pin
// `nself update` moved), is an error, not a container that crash-loops.
func (g *Generator) authImage() (string, error) {
	version := strings.TrimSpace(g.cfg.Auth.Version)
	entry, keyMissing := AuthEntry(g.cfg.Passthrough)
	v14, _ := LockedRef("auth-v14")
	cur, _ := LockedRef("auth")
	switch {
	case v14.IsDefaultVersion(version):
		version = ""
	case cur.IsDefaultVersion(version): // the current release, but never without its key
		version, entry, keyMissing = "", "auth", false
		if !AuthKeySet(g.cfg.Passthrough) {
			entry, keyMissing = "auth-v14", true
		}
	}
	image := ""
	switch {
	case version != "" && !authTagNeedsKey(version):
		// A moving tag (latest) or a release before 0.43.0 keeps the exact string it
		// rendered before the rename: nhost/auth lacks some of those tags (0.42.0),
		// and latest there would be 0.52+ (lens and adversarial reviews, cli#702).
		return frozenAuthRepo + ":" + version, nil
	case version != "":
		if !AuthKeySet(g.cfg.Passthrough) {
			noticeAuth("AUTH_VERSION=%s needs %s (64 hex characters); auth will not start without it", version, AuthEncryptionKeyEnv)
		}
	default:
		pin := ProjectPins()["auth"]
		if pin != "" && repoOf(pin) == frozenAuthRepo {
			image = pin
		} else if pin == "" && keyMissing && entry == "auth-v14" {
			noticeAuth("auth stays on nhost/auth 0.36.0 until %s is set (64 hex characters); the current release requires it", AuthEncryptionKeyEnv)
		}
	}
	switch {
	case image != "":
	case entry == "auth":
		image = ResolveImage("auth", ImageRef("auth", version))
	default:
		image = ResolveImage("auth", ImageRef("auth-v14", version))
	}
	if version == "" && AuthRefNeedsKey(image) && !AuthKeySet(g.cfg.Passthrough) {
		return "", fmt.Errorf("auth image %s needs %s (64 hex characters) in the env: put back the key this project ran with (.env.secrets); a database it migrated cannot go back to 0.36.0 without restoring the pre-upgrade backup (Image-Lock wiki page, Roll back)", imageBase(image), AuthEncryptionKeyEnv)
	}
	return image, nil
}

var ( // authNoticeW receives the auth notice, once per process (authNoticed)
	authNoticeW io.Writer = os.Stderr
	authNoticed atomic.Bool
)

// noticeAuth prints one auth notice per process; callers never pass the key.
func noticeAuth(format string, args ...any) {
	if !authNoticed.Swap(true) && authNoticeW != nil {
		_, _ = fmt.Fprintf(authNoticeW, "notice: "+format+"\n", args...)
	}
}
