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
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

func init() {
	for _, c := range []*cobra.Command{accountCmd, accountStatusCmd, accountDevicesCmd, accountLicensesCmd, accountTeamCmd} {
		c.RunE = accountEnv(c.RunE)
	}
}

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

// accountLicenseRows is the licence list for a document: no slice is nil, so
// a reply that omits a licence's bundles prints [] not null.
func accountLicenseRows(in []auth.LicenseInfo) []auth.LicenseInfo {
	out := orEmpty(in)
	for i := range out {
		out[i].Bundles = orEmpty(out[i].Bundles)
	}
	return out
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
	return emitDoc(cmd, doc, true)
}

// accountCachedLegacy is the bare `account status --json` answer served from
// the auth file before the v1 contract: no status member, cached=true. Fields
// stay in key order because the old output was a sorted map.
type accountCachedLegacy struct {
	Bundles     []string `json:"bundles"`
	Cached      bool     `json:"cached"`
	DisplayName string   `json:"display_name"`
	Email       string   `json:"email"`
	Tier        string   `json:"tier"`
}

// emitAccountCachedStatus writes the cached status: the v1 document, or the
// pre-contract bare shape under NSELF_JSON_LEGACY (D-0980).
func emitAccountCachedStatus(cmd *cobra.Command, af *auth.AuthFile) error {
	doc := accountCachedStatus(af, "cached")
	if output.LegacyBare() {
		return emitDoc(cmd, accountCachedLegacy{Bundles: af.Bundles, Cached: true, DisplayName: doc.DisplayName,
			Email: doc.Email, Tier: doc.Tier}, true)
	}
	return emitAccountStatus(cmd, doc)
}
