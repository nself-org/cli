package commands

// Purpose: the auth row of `nself update` (D-0661, P7-LIVE-20). The auth service
// has two image lock entries on nhost/auth (auth-v14 = 0.36.0, auth = the
// current release, which needs AUTH_ENCRYPTION_KEY and migrates the database one
// way), so the generic per-repository lookup cannot choose between them.
// Inputs: the project's auth pin and AUTH_* passthrough (config).
// Outputs: the plan candidate for the auth row; a preflight error before --yes
// changes anything when the build would refuse the pinned release.
// Constraints: never moves a pin that migrated the database back, never names
// the key's value.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/upgrade/plan"
)

// authCandidate is the auth row (D-0661): the entry compose.AuthEntry picks for
// the mode and AUTH_ENCRYPTION_KEY, never a step back from a release that has
// migrated the database (that needs the pre-upgrade backup restored), and never a
// move of a digest-only pin whose release the lock cannot name.
func authCandidate(pin string, passthrough map[string]string) plan.Candidate {
	entry, keyMissing := compose.AuthEntry(passthrough)
	r, ok := compose.LockedRef(entry)
	v14, _ := compose.LockedRef("auth-v14")
	if !ok {
		return plan.Candidate{Reason: plan.ReasonNoImage}
	}
	base, digest, _ := strings.Cut(pin, "@")
	tagless := strings.LastIndex(base, ":") <= strings.LastIndex(base, "/")
	switch {
	case tagless && digest != v14.IndexDigest && !compose.AuthRefNeedsKey(pin):
		return plan.Candidate{Reason: "the auth pin names a digest the CLI image lock does not know, so update cannot tell whether it migrated the database; it is left as is"}
	case compose.AuthRefNeedsKey(pin) && !compose.AuthRefNeedsKey(r.String()) && keyMissing:
		return plan.Candidate{Reason: "the pinned auth release needs AUTH_ENCRYPTION_KEY: put back the key this project ran with; going back to 0.36.0 needs the pre-upgrade backup restored"}
	case compose.AuthRefNeedsKey(pin) && !compose.AuthRefNeedsKey(r.String()):
		return plan.Candidate{Reason: "the pinned auth release has migrated the database one way; going back needs the pre-upgrade backup restored (Image-Lock wiki, Roll back)"}
	case !compose.AuthRefNeedsKey(pin) && compose.AuthRefNeedsKey(r.String()):
		return plan.Candidate{Value: r.String(), Note: "auth " + r.Version + " migrates the auth schema and encrypts TOTP secrets one way on first start: take a verified backup first"}
	}
	return plan.Candidate{Value: r.String()}
}

// authKeyPreflight stops `nself update --yes` before any local change when the
// project's auth pin is a release that needs AUTH_ENCRYPTION_KEY and the key is
// missing or malformed: the rebuild would refuse it after plugins were already
// installed, and restoring the lock does not uninstall them.
func authKeyPreflight(l lock.Lock) error {
	pin := l.Images["auth"]
	if pin == "" || !compose.AuthRefNeedsKey(pin) {
		return nil
	}
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("reading AUTH_ENCRYPTION_KEY for the update: %w", err)
	}
	if compose.AuthKeySet(cfg.Passthrough) {
		return nil
	}
	return fmt.Errorf("the auth pin %s needs %s (64 hex characters) and the env has no valid one: put back the key this project ran with, then run nself update --yes again (nothing was changed)", strings.SplitN(pin, "@", 2)[0], compose.AuthEncryptionKeyEnv)
}
