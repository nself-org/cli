package canon

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ValidationError aggregates every problem found in one validation pass. Each
// problem names the offending path (for example `commands["config get"]`).
type ValidationError struct {
	// Problems are sorted, one per violated rule instance.
	Problems []string
}

// NewValidationError returns nil for no problems, else a *ValidationError with
// the problems sorted so the message is deterministic.
func NewValidationError(problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	p := append([]string(nil), problems...)
	sort.Strings(p)
	return &ValidationError{Problems: p}
}

// Error renders every problem, one per line.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("canon: %d problem(s):\n  - %s", len(e.Problems), strings.Join(e.Problems, "\n  - "))
}

// Depth returns the number of words in a command key ("config get" is 2).
func Depth(key string) int { return len(strings.Fields(key)) }

// Validate checks every rule that needs only the declared data. Rules that
// need the cobra tree live in internal/cmdregistry.Build. It returns nil or a
// *ValidationError.
func (f *File) Validate() error {
	var p []string
	p = append(p, f.validateHeader()...)
	verbs := map[string]bool{}
	for _, v := range f.Verbs {
		verbs[v] = true
	}
	for key, e := range f.Commands {
		p = append(p, validateEntry(key, e, verbs)...)
	}
	p = append(p, f.validate("")...)
	return NewValidationError(p)
}

func (f *File) validateHeader() []string {
	var p []string
	if f.SchemaVersion != SchemaVersion {
		p = append(p, fmt.Sprintf("schema_version: %d is not supported (want %d)", f.SchemaVersion, SchemaVersion))
	}
	if len(f.Verbs) == 0 {
		p = append(p, "verbs: must list the ADR 0016 core verbs")
	}
	if len(f.Verbs) > MaxVerbs {
		p = append(p, fmt.Sprintf("verbs: %d verbs exceeds the ceiling of %d (ADR 0016)", len(f.Verbs), MaxVerbs))
	}
	seen := map[string]bool{}
	for _, v := range f.Verbs {
		if v == "" || strings.ContainsAny(v, " \t") {
			p = append(p, fmt.Sprintf("verbs: %q is not a single word", v))
		}
		if seen[v] {
			p = append(p, fmt.Sprintf("verbs: %q is listed twice", v))
		}
		seen[v] = true
	}
	return p
}

func validateEntry(key string, e Entry, verbs map[string]bool) []string {
	at := fmt.Sprintf("commands[%q]", key)
	if key == "" || key != strings.Join(strings.Fields(key), " ") {
		return []string{at + ": key must be a command path of single-space-separated words without the leading \"nself \""}
	}
	var p []string
	depth := Depth(key)
	cn := e.Canon
	switch {
	case cn == "" && depth >= 2:
		cn = CanonSubcommand
	case cn == "":
		p = append(p, at+": canon is required for a top-level command (core, pending, deprecated-shim or builtin)")
	case !ValidCanon(cn):
		p = append(p, fmt.Sprintf("%s: canon %q is not one of core, subcommand, plugin, deprecated-shim, pending, builtin", at, cn))
	case cn == CanonPlugin:
		p = append(p, at+": canon plugin is reserved for plugin-mounted commands and is rejected on cobra-native commands")
	case depth == 1 && cn == CanonSubcommand:
		p = append(p, at+": canon subcommand needs depth >= 2")
	case depth >= 2 && cn != CanonSubcommand && cn != CanonShim:
		p = append(p, fmt.Sprintf("%s: canon %q is not allowed at depth >= 2 (subcommand or deprecated-shim)", at, cn))
	}
	if depth == 1 && ValidCanon(cn) {
		if cn == CanonCore && !verbs[key] {
			p = append(p, at+": canon core but the name is not in verbs")
		}
		if cn != CanonCore && verbs[key] {
			p = append(p, fmt.Sprintf("%s: the name is in verbs so canon must be core, not %q", at, cn))
		}
	}
	if (cn == CanonShim) != (e.Target != "") {
		if cn == CanonShim {
			p = append(p, at+": deprecated-shim requires target")
		} else {
			p = append(p, at+": target is only allowed on a deprecated-shim")
		}
	}
	if e.Target == key {
		p = append(p, at+": target must not be the command itself")
	}
	p = append(p, validateEnums(at, e)...)
	p = append(p, validateExitCodes(at+" exit_codes", e.ExitCodes)...)
	p = append(p, validateExitCodes(at+" exit_codes_v15", e.ExitCodesV15)...)
	for name, ov := range e.Flags {
		p = append(p, validateOverride(fmt.Sprintf("%s flag %q", at, name), name, ov)...)
	}
	return p
}

func validateEnums(at string, e Entry) []string {
	var p []string
	if e.SideEffect != "" && Rank(e.SideEffect) < 0 {
		p = append(p, fmt.Sprintf("%s: side_effect %q is not one of read, write, remote, destructive", at, e.SideEffect))
	}
	if e.Output != "" && !ValidOutput(e.Output) {
		p = append(p, fmt.Sprintf("%s: output %q is not one of document, stream, interactive", at, e.Output))
	}
	switch e.JSON {
	case "", JSONLegacy, JSONNone:
	case JSONEnvelope:
		p = append(p, at+": json envelope may not be declared in YAML (it is derived from the registered data types)")
	default:
		p = append(p, fmt.Sprintf("%s: json %q is not one of legacy, none", at, e.JSON))
	}
	return p
}

func validateOverride(at, name string, ov FlagOverride) []string {
	var p []string
	if name == "" || strings.HasPrefix(name, "-") {
		p = append(p, at+": flag name must be the bare long name without dashes")
	}
	if ov.SideEffect != "" && Rank(ov.SideEffect) < 0 {
		p = append(p, fmt.Sprintf("%s: side_effect %q is not one of read, write, remote, destructive", at, ov.SideEffect))
	}
	switch ov.JSON {
	case "", JSONLegacy, JSONNone:
	case JSONEnvelope:
		p = append(p, at+": json envelope may not be declared in YAML (it is derived from the registered data types)")
	default:
		p = append(p, fmt.Sprintf("%s: json %q is not one of legacy, none", at, ov.JSON))
	}
	if ov.Output != "" && ov.Output != OutputStream {
		p = append(p, fmt.Sprintf("%s: output %q is invalid (the only flag-level value is stream)", at, ov.Output))
	}
	return p
}

func validateExitCodes(at string, m map[string]string) []string {
	var p []string
	for k := range m {
		n, err := strconv.Atoi(k)
		if err != nil || strconv.Itoa(n) != k || n < 0 || n > 125 {
			p = append(p, fmt.Sprintf("%s: key %q must be an integer 0-125 written without sign or leading zeros", at, k))
		}
	}
	return p
}
