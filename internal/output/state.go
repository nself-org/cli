package output

import "sync"

// state is the per-process output state: the resolved invocation, the recorded
// meta notices and whether the legacy-mode warning was already printed. One
// mutex guards it; callers never see the fields.
var state struct {
	mu          sync.Mutex
	command     string
	json        bool
	known       bool
	deprecation []Deprecation
	warnings    []string
	legacyWarn  bool
}

// SetInvocation records the resolved command path (without "nself ", "" when
// none) and whether JSON mode is on, so a failure reported later by main knows
// how to render. Calling it again replaces the earlier value.
func SetInvocation(command string, json bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.command, state.json, state.known = command, json, true
}

// Invocation returns what SetInvocation recorded. known is false when no
// invocation was recorded yet; callers then fall back to
// JSONRequestedFromArgs on the raw arguments.
func Invocation() (command string, json bool, known bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.command, state.json, state.known
}

// AddDeprecation records a deprecation notice for the envelope meta member.
// A notice identical to one already recorded is ignored; order of first
// insertion is kept.
func AddDeprecation(old, new, removalAt string) {
	d := Deprecation{Old: old, New: new, RemovalAt: removalAt}
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, have := range state.deprecation {
		if have == d {
			return
		}
	}
	state.deprecation = append(state.deprecation, d)
}

// AddWarning records a warning for the envelope meta member. A message already
// recorded is ignored; order of first insertion is kept.
func AddWarning(msg string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, have := range state.warnings {
		if have == msg {
			return
		}
	}
	state.warnings = append(state.warnings, msg)
}

// ResetState clears the invocation, the recorded meta notices and the
// once-per-process legacy warning. Tests call it between cases; a host that
// serves many invocations in one process (MCP, HTTP) calls it between them.
func ResetState() {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.command, state.json, state.known = "", false, false
	state.deprecation, state.warnings, state.legacyWarn = nil, nil, false
}

// currentMeta returns a copy of the recorded notices, or nil when there are
// none so the envelope carries no meta key.
func currentMeta() *Meta {
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.deprecation) == 0 && len(state.warnings) == 0 {
		return nil
	}
	m := &Meta{}
	if len(state.deprecation) > 0 {
		m.Deprecations = append([]Deprecation(nil), state.deprecation...)
	}
	if len(state.warnings) > 0 {
		m.Warnings = append([]string(nil), state.warnings...)
	}
	return m
}

// claimLegacyWarning reports true exactly once per process (until
// ResetState): the caller that gets true prints the legacy-mode warning.
func claimLegacyWarning() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.legacyWarn {
		return false
	}
	state.legacyWarn = true
	return true
}
