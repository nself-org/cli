// JSON data-type registration for the observe domain (status, doctor).
//
// Purpose: P7-REG-09 pilots: the unchanged pre-contract payload plus `state`.
// Pre-contract bare JSON in v1.4 mode, envelope in v1.5 (EPIC D8).
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

func init() {
	registerJSONType("status", statusData{})
	registerJSONType("doctor", doctorData{})
}
