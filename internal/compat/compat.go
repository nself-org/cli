// Package compat is the single switch ADR 0021 requires for every P7 change
// that breaks an existing user-visible behaviour.
//
// The rule: patch releases keep shipping from main, so a breaking change lands
// dormant. The old (v1.4) behaviour stays the default; the new (v1.5)
// behaviour runs only when V15 reports true. There is exactly one switch, one
// environment variable (EnvVar) and one marker format. A per-feature flag, a
// second switch or a build tag for P7 behaviour is not allowed.
//
// V15 is true when NSELF_V15 is "1" or "true" (any case). Unset, empty, "0"
// and any other value are false. The value is read from the environment on
// every call and never cached, so tests flip it with t.Setenv.
//
// Marker format: the line directly above (or the same line as) every gated
// call to V15 carries
//
//	// compat.V15(<ticket-id>): <old behaviour> -> <new behaviour>
//
// scripts/ci/compat-markers.sh lists every marker as "<file>:<line>
// <ticket-id>", fails when a call has no marker, and renders the "Gated
// behaviours" table of .github/wiki/Compat-V15.md from the comments.
//
// P7-SHIP flips the default to v1.5 and deletes the old branches.
//
// Layer: L0. This package imports the standard library only.
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
