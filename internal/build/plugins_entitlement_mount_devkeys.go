//go:build nself_devkeys

package build

import "os"

// Purpose: permit local ping boot kits to omit runtime mounts only in dev builds.
// Inputs: override presence. Outputs: tagged skip policy with one warning.
// Constraints: never compiled into releases. SPORT: P7-PLUG-24 / D9.
func init() {
	skipEntitlementMount = func() bool {
		if os.Getenv("LICENSE_PUBLIC_KEY_OVERRIDE") == "" {
			return false
		}
		warnOnce("development key override: skipping plugin entitlement mount")
		return true
	}
}
