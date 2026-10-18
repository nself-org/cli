package commands

// Purpose: the v1 envelope helpers of the account commands (`license account`
// status, devices, licenses, team; P7-SURF-16).
// Inputs: the auth file and the account server's lists. Outputs: the
// legacy-compatible document through emitEnv (the pre-contract JSON is bare in
// v1.4, an envelope in v1.5).
// Constraints: the access and session tokens never reach a document. The
// mutation flags (--revoke, --activate, --invite, --remove, --role) answer
// with text and a prompt, not a list, so --json with one of them is refused
// with E402 in v1.5 mode; v1.4 keeps its behaviour.
// SPORT: P7-SURF-16, contract:cli.json-envelope

import (
	"fmt"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

// orEmpty returns s, or an empty non-nil slice so a document prints [] not null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// accountJSONGuard refuses --json (E402) when one of the mutation flags is set
// and the invocation answers with an envelope.
func accountJSONGuard(cmd *cobra.Command, flags ...string) error {
	if !jsonEnvelopeOn(cmd) {
		return nil
	}
	for _, name := range flags {
		if cmd.Flags().Changed(name) {
			return errs.New("E402", fmt.Sprintf("nself %s --%s does not support --json", invokedKey(cmd), name))
		}
	}
	return nil
}

// accountCachedStatus is the status document built from the auth file only.
func accountCachedStatus(af *auth.AuthFile, status string) accountStatusData {
	tier := af.Tier
	if tier == "" {
		tier = "free"
	}
	return accountStatusData{Status: status, Email: af.Email, DisplayName: af.DisplayName, Tier: tier, Bundles: orEmpty(af.Bundles)}
}

// accountLiveStatus is the status document of an active session.
func accountLiveStatus(af *auth.AuthFile, email, name, tier string, verified, mfa bool) accountStatusData {
	if tier == "" {
		tier = "free"
	}
	return accountStatusData{Status: "active", Email: email, DisplayName: name, Tier: tier,
		EmailVerified: &verified, MFAEnabled: &mfa, Bundles: orEmpty(af.Bundles)}
}

// emitAccountStatus writes a status document.
func emitAccountStatus(cmd *cobra.Command, doc accountStatusData) error {
	return emitEnv(cmd, doc, true)
}
