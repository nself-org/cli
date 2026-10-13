//go:build nself_devkeys

package entitlement

import "os"

// DevKeyMode identifies the explicit local ping exception in development builds.
// Purpose: permit local ping. Inputs: override environment. Outputs: boolean.
// Constraints: compiled only under nself_devkeys. SPORT: development entitlement gate.
func DevKeyMode() bool { return os.Getenv("LICENSE_PUBLIC_KEY_OVERRIDE") != "" }
