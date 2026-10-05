package commands

import (
	"os"
	"strings"
)

// Retired top-level spellings, and how they map onto the consolidated tree.
//
// Purpose: CLI-R09 folds four `release*` commands, a stray `pitr`, `dns-setup`,
// `migrate-from-v099`, `ollama` and the ambiguous `feature`/`flag` pair into
// their parents. Every old invocation must keep working exactly as before.
//
// Inputs: os.Args, rewritten in place before cobra parses anything.
//
// Outputs: the canonical argv, plus a deprecation warning on stderr naming the
// new spelling.
//
// Constraints: argv rewriting is used rather than hidden wrapper commands on
// purpose. A wrapper that borrows another command's RunE does not inherit its
// flags — `nself up --allow-legacy` would have failed to parse under the
// wrappers removed in CLI-R03. Splicing argv keeps every flag, positional
// argument and completion behaviour identical, because the same cobra.Command
// ends up handling the call.
//
// This must run before the unknown-command plugin proxy in Execute(): once a
// name is no longer a registered top-level command, the proxy would otherwise
// try to resolve it as a plugin.

// legacySpelling describes one retired invocation.
type legacySpelling struct {
	// canonical is the argv path that replaces the retired name.
	canonical []string
	// note is appended to the deprecation warning when the mapping is not a
	// pure rename (for example when a command moved under a different parent
	// because two implementations shared a name).
	note string
}

// legacySpellings maps a retired top-level command name to its new home. The
// deprecation registry (internal/deprecation/registry.yaml) carries the
// user-facing message; this table carries the mechanics.
var legacySpellings = map[string]legacySpelling{
	"release-check":     {canonical: []string{"release", "check"}},
	"release-rollback":  {canonical: []string{"release", "rollback"}},
	"release-status":    {canonical: []string{"release", "status"}},
	"migrate-from-v099": {canonical: []string{"migrate", "from-v099"}},
	"dns-setup":         {canonical: []string{"trust", "dns"}},
	"ollama":            {canonical: []string{"model", "ollama"}},
	"feature":           {canonical: []string{"config", "features"}},
	"ssl":               {canonical: []string{"trust", "ssl"}},
	"upgrade":           {canonical: []string{"update", "upgrade"}},
	"pitr": {
		canonical: []string{"backup", "pitr"},
		note: "note: `nself db pitr` is a different implementation covering " +
			"database-side PITR configuration; this one manages WAL archiving and base backups",
	},
}

// rewriteLegacyInvocation rewrites os.Args when the user typed a retired
// spelling, and reports the spelling that was rewritten (empty when nothing
// changed) so the caller can emit the deprecation warning.
//
// Only os.Args[1] is considered: these are all top-level renames, and a
// deeper match would risk rewriting a positional argument that happens to
// share the name.
func rewriteLegacyInvocation() string {
	if len(os.Args) < 2 {
		return ""
	}
	name := os.Args[1]
	entry, ok := legacySpellings[name]
	if !ok {
		return ""
	}

	rewritten := make([]string, 0, len(os.Args)+len(entry.canonical))
	rewritten = append(rewritten, os.Args[0])
	rewritten = append(rewritten, entry.canonical...)
	rewritten = append(rewritten, os.Args[2:]...)
	os.Args = rewritten

	return name
}

// warnLegacySpelling writes the deprecation warning for a rewritten spelling.
// The registry is the source of the message so the wording stays consistent
// with every other deprecation; this only adds the mapping-specific note.
func warnLegacySpelling(name string) {
	if name == "" || deprecationRegistry == nil {
		return
	}
	// Scripted callers opt out with the same flag cobra exposes. The flag is
	// not parsed yet at this point, so argv is checked directly.
	if warningsSilenced(os.Args) {
		return
	}

	item, ok := deprecationRegistry.Lookup("nself " + name)
	if !ok {
		return
	}
	deprecationRegistry.Warn(os.Stderr, item)

	if note := legacySpellings[name].note; note != "" {
		if _, err := os.Stderr.WriteString(strings.TrimRight(note, "\n") + "\n"); err != nil {
			return
		}
	}
}

// warnLegacyChain emits the warning for a legacy spelling (empty when the user
// typed none) and the canon rewrites that followed it.
//
// v1.4 has no notes, so a legacy spelling warns exactly as before. In v1.5 a
// legacy spelling whose target moved produces ONE warning naming the final
// canonical path (`dns-setup` -> `config trust dns`), not the v1.4 spelling the
// registry item names; registry.yaml replacement strings are untouched.
func warnLegacyChain(legacy string, notes []canonNote) {
	switch {
	case len(notes) == 0 && legacy != "":
		warnLegacySpelling(legacy)
	case len(notes) > 0 && legacy == "":
		emitCanonWarning(os.Stderr, os.Args[1:], notes, "", "")
	case len(notes) > 0:
		// compat.V15(P7-CANON-21): the legacy warning names the v1.4 spelling the registry item carries -> one warning names the final canonical path
		n, canonical := notes[0], legacySpellings[legacy].canonical
		final := n.New
		if len(canonical) >= len(n.From) {
			final = strings.Join(append(append([]string{}, n.To...), canonical[len(n.From):]...), " ")
		}
		emitCanonWarning(os.Stderr, os.Args[1:], notes, legacy, final)
	}
}
