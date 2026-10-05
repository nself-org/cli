package controlplane

// prodclass.go: which deploy environments count as production for the
// deploy confirmation gate.
//
// Purpose: one definition of "prod-class" shared by the deploy command and the
// pipeline, so a custom environment name cannot slip past the production gate.
// Inputs:  the loaded inventory (may be nil) and an environment name.
// Outputs: true when the environment must be treated as production.
// Constraints: today the rule is the name (prod or production, any case).
//   P7-DEPL-13 adds the inventory tier (tier: prod) to this function; callers
//   must keep passing the inventory so that change needs no caller edits. The
//   check is conservative by construction: an unknown or empty name is not
//   prod-class here, so callers resolve the name against the inventory first
//   and refuse unknown names before asking this question.

import "strings"

// IsProdClass reports whether env names a production-class environment.
func IsProdClass(inv *Inventory, env string) bool {
	_ = inv // tier lookup arrives with P7-DEPL-13
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return true
	}
	return false
}
