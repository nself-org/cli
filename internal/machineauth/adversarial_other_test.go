//go:build !unix

package machineauth

// Purpose: placeholder for the unix-only adversarial subtests on other platforms.
// Inputs: none. Outputs: an empty subtest list.
// Constraints: keeps TestMachineAuthAdversarial buildable everywhere.

import "testing"

var unixSubtests = []struct {
	name string
	fn   func(*testing.T)
}{}
