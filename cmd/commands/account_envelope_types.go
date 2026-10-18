package commands

// Purpose: the typed v1 envelope data of the account commands (P7-SURF-16).
// Inputs: the auth file and the account server's answers. Outputs: Go types
// registered in registry_types_account.go. Devices, licences and team members
// keep the pre-contract array shape (auth.DeviceEntry, auth.LicenseInfo,
// auth.TeamMember), which carries no token.
// Constraints: the access and session tokens never reach a document.
// SPORT: P7-SURF-16, contract:cli.json-envelope

// accountStatusData is the data of `license account status` (and of
// `license account`): the pre-contract document plus Status.
type accountStatusData struct {
	// Status is active, cached (the server was unreachable), expired or not-logged-in.
	Status string `json:"status"`
	// Email is the account email, "" when not logged in.
	Email string `json:"email"`
	// DisplayName is the account display name, "" when none.
	DisplayName string `json:"display_name"`
	// Tier is the account tier ("free" when unset), "" when not logged in.
	Tier string `json:"tier"`
	// EmailVerified is present only for an active session.
	EmailVerified *bool `json:"email_verified,omitempty"`
	// MFAEnabled is present only for an active session.
	MFAEnabled *bool `json:"mfa_enabled,omitempty"`
	// Bundles are the bundles stored with the login.
	Bundles []string `json:"bundles"`
}
