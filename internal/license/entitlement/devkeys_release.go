//go:build !nself_devkeys

package entitlement

// DevKeyMode identifies the local ping exception, unavailable in release builds.
// Purpose: preserve release enforcement. Inputs: none. Outputs: false.
// Constraints: default builds ignore all key override variables. SPORT: devkeys gate.
func DevKeyMode() bool { return false }
