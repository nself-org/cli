package cmdlog

// Purpose: flag-name and URL rules for the command log (D-0473).
// Inputs: one argv element and the element after it.
// Outputs: whether a flag name carries a secret value; an element with URL
// credentials replaced.
// Constraints: no second rule here. Secret names go through
// config.IsSecretKey and URL credentials through config.MaskURLCredentials
// (internal/config/secretkeys.go, SPORT cap:cli.machine-auth). internal/config
// does not import this package, so there is no cycle. The one addition is
// AUTH on flag names (see secretFlagName). Matching over-masks (--keyboard
// and --author hide their value); that is the safe side.

import (
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// secretFlagName reports whether a long flag name (with its leading dashes and
// no "=value") names a secret: the config secret-name rule plus AUTH. argv
// carries bearer and authorization values under names that are not env keys
// (--auth, --authorization, --auth-header), so the command log adds AUTH on
// top of the canon rule. config stays unchanged so env display and the audit
// log do not widen.
func secretFlagName(name string) bool {
	if !strings.HasPrefix(name, "--") {
		return false
	}
	bare := strings.TrimLeft(name, "-")
	return config.IsSecretKey(bare) || strings.Contains(strings.ToUpper(bare), "AUTH")
}

// maskURLCredentials masks the credentials of every URL value in s by the
// config rule (user and password become ****, secret query params too).
func maskURLCredentials(s string) string {
	return config.MaskURLCredentials(s)
}
