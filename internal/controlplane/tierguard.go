package controlplane

// tierguard.go: the explicit non-prod guard for write operations that must
// never reach production (preview, teardown, copy-down).
//
// Purpose: one fail-closed check those callers run before any write.
// Inputs:  the inventory (may be nil) and an environment name.
// Outputs: nil, or a coded error naming the environment and the reason.
// Constraints: pure, no I/O. A tier derived by DeriveTier is not enough; the
//   environment must carry an explicit tier field of local or local-servers,
//   and IsProdClass must be false. There is no override.

import (
	"fmt"

	"github.com/nself-org/cli/internal/errs"
)

// RequireExplicitNonProd returns nil only when env is in inv, has an explicit
// tier of local or local-servers, and is not prod-class. Every other case,
// including a nil inventory, an unknown env and an empty tier, is an error.
func RequireExplicitNonProd(inv *Inventory, env string) error {
	if inv == nil {
		return errs.New("E483", fmt.Sprintf("environment %q: no deploy inventory loaded; refusing a non-prod write", env))
	}
	e, ok := inv.Environments[env]
	if !ok {
		return errs.New("E483", fmt.Sprintf("environment %q: not in the deploy inventory; refusing a non-prod write", env))
	}
	switch e.Tier {
	case "":
		return errs.New("E485", fmt.Sprintf("environments.%s.tier: no explicit tier; a derived tier is not enough for this write", env))
	case TierLocal, TierLocalServers:
	default:
		return errs.New("E485", fmt.Sprintf("environments.%s.tier: tier %q is not local or local-servers; refusing a non-prod write", env, e.Tier))
	}
	if IsProdClass(inv, env) {
		return errs.New("E485", fmt.Sprintf("environments.%s: prod-class environment; refusing a non-prod write", env))
	}
	return nil
}
