// JSON data-type registration for the account domain (license, license
// account, license login/logout).
//
// Purpose: every runnable document command of the account canon fragment
// answers with the v1 envelope in v1.5 mode (P7-SURF-16). The command group
// answers with its subcommands (dataHub); read commands with typed facts
// (license_envelope_types.go, account_envelope_types.go); `license clear` and
// `license import` with the shared dataResult. Commands that printed JSON
// before the contract (`license show`, `license health`, `license account
// status|devices|licenses|team`) keep that output in v1.4 and say so in the
// v1.4 registry view (`json: legacy` in account.yaml, kept by the cmdregistry
// mapping for V15-only rows); all rows are v1.5-only envelopes.
// `license tail` is a stream; `license login`, `license logout`, `license
// account login|logout|transfer` are interactive or credential-changing and
// register nothing (account.yaml carries the reasons).
// Constraints: registers from init(); see registry_types.go for the rules. A
// licence key is masked (license.MaskKey) in every type registered here.

package commands

import "github.com/nself-org/cli/internal/auth"

// accountEnvelopeTypes maps each path of the account fragment to its envelope data.
var accountEnvelopeTypes = map[string]any{
	"license":                  dataHub{},
	"license account":          accountStatusData{},
	"license account devices":  []auth.DeviceEntry{},
	"license account licenses": []auth.LicenseInfo{},
	"license account status":   accountStatusData{},
	"license account team":     []auth.TeamMember{},
	"license add":              licenseAddData{},
	"license clear":            dataResult{},
	"license export":           licenseExportData{},
	"license health":           licenseHealthJSON{},
	"license import":           dataResult{},
	"license list":             licenseStatusData{},
	"license migrate":          licenseMigrateData{},
	"license refresh":          licenseRefreshData{},
	"license remove":           licenseRemoveData{},
	"license restore":          licenseRestoreData{},
	"license revalidate":       licenseRevalidateData{},
	"license revoke":           licenseRevokeData{},
	"license set":              licenseSetData{},
	"license show":             licenseShowData{},
	"license simulate-offline": licenseSimulateData{},
	"license status":           licenseStatusData{},
	"license upgrade":          licenseUpgradeData{},
	"license validate":         licenseValidateData{},
}

func init() {
	for path, zero := range accountEnvelopeTypes {
		registerJSONType(path, zero)
		registerV15OnlyEnvelope(path)
	}
}
