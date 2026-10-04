package canon

// SchemaVersion is the only canon.yaml schema version this package accepts.
const SchemaVersion = 1

// MaxVerbs is the ceiling on the core verb list (ADR 0016, VMI). It is a
// ceiling, not a target: the list may hold fewer verbs, never more.
const MaxVerbs = 20

// Canon status values (Epic P7-REG D4).
const (
	// CanonCore marks one of the ADR 0016 verbs.
	CanonCore = "core"
	// CanonSubcommand marks a command at depth >= 2 (the default there).
	CanonSubcommand = "subcommand"
	// CanonPlugin is reserved for plugin-mounted commands; no cobra-native
	// command may carry it.
	CanonPlugin = "plugin"
	// CanonShim marks a hidden alias kept for the deprecation window; its
	// Target names the canonical path.
	CanonShim = "deprecated-shim"
	// CanonPending marks a top-level command outside the canon whose
	// disposition P7-CANON decides.
	CanonPending = "pending"
	// CanonBuiltin marks framework-provided commands (help); not counted
	// against the canon.
	CanonBuiltin = "builtin"
)

// Side-effect classes (D5), in total order read < write < remote < destructive.
const (
	SideEffectRead        = "read"
	SideEffectWrite       = "write"
	SideEffectRemote      = "remote"
	SideEffectDestructive = "destructive"
)

// Output kinds (D6).
const (
	OutputDocument    = "document"
	OutputStream      = "stream"
	OutputInteractive = "interactive"
)

// JSON support values (D7).
const (
	JSONEnvelope = "envelope"
	JSONLegacy   = "legacy"
	JSONNone     = "none"
)

// sideEffectOrder is the D5 total order, lowest first.
var sideEffectOrder = []string{SideEffectRead, SideEffectWrite, SideEffectRemote, SideEffectDestructive}

// Rank returns the position of a side-effect class in the total order
// read(0) < write(1) < remote(2) < destructive(3), or -1 for an unknown value.
func Rank(sideEffect string) int {
	for i, s := range sideEffectOrder {
		if s == sideEffect {
			return i
		}
	}
	return -1
}

// ValidCanon reports whether s is one of the six canon status values.
func ValidCanon(s string) bool {
	switch s {
	case CanonCore, CanonSubcommand, CanonPlugin, CanonShim, CanonPending, CanonBuiltin:
		return true
	}
	return false
}

// ValidOutput reports whether s is one of the three output kinds.
func ValidOutput(s string) bool {
	return s == OutputDocument || s == OutputStream || s == OutputInteractive
}

// ValidJSON reports whether s is one of the three JSON support values.
func ValidJSON(s string) bool {
	return s == JSONEnvelope || s == JSONLegacy || s == JSONNone
}
