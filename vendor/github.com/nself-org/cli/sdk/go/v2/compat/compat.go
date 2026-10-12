// Package compat is the plugin-module copy of the ADR 0021 switch: the one
// rule that decides whether a breaking P7 change runs its v1.5 behaviour.
//
// Plugins are separate Go modules and cannot import the CLI's internal/compat,
// so this package implements the same rule. The two copies are pinned together
// by one golden table, testdata/rule.json, which both packages' tests read.
//
// The rule: patch releases keep shipping from main, so a breaking change lands
// dormant. The old (v1.4) behaviour stays the default; the new (v1.5) behaviour
// runs only when V15 reports true. V15 is true when NSELF_V15 is "1" or "true"
// in any case. Unset, empty, "0" and every other value are false. The value is
// read from the environment on every call and never cached.
//
// Marker format: the line directly above (or the same line as) every gated
// call carries
//
//	// compat.V15(<ticket-id>): <old behaviour> -> <new behaviour>
//
// P7-SHIP-09 flips the CLI default to v1.5; the CLI exports the resolved mode to
// plugin processes (contract:cli.plugin-exec-env), so a plugin started by the
// CLI sees the same answer the CLI computed. The cli column of rule.json is the
// only part that changes at that point.
//
// This package imports the standard library only.
package compat

import (
	"os"
	"strings"
)

// EnvVar is the environment variable that selects v1.5 behaviour.
const EnvVar = "NSELF_V15"

// V15 reports whether v1.5 behaviour is selected: true iff NSELF_V15 is "1" or
// "true" in any case. It reads the environment on every call.
func V15() bool {
	v := os.Getenv(EnvVar)
	return v == "1" || strings.EqualFold(v, "true")
}

// Mode returns the active compat mode as a string: "v1.5" when V15 is true,
// otherwise "v1.4".
func Mode() string {
	if V15() {
		return "v1.5"
	}
	return "v1.4"
}
