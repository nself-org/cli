package cmdregistry

// bindings.go — fills Flag.env from the declared config bindings.
//
// Purpose:     the registry reports, per flag, the configuration key the flag
//              supplies (contract:config.env, P7-SURF-06). The declaration is
//              internal/config/bindings; this file only applies it.
// Inputs:      a built *Registry, the bindings keyed by canonical (v1.5)
//              command path then flag name, the canon moves rows, the compat
//              mode.
// Outputs:     Flag.env set on the bound flags; nothing else changes.
// Constraints: stdlib only (the bindings package is not imported: the caller
//              passes plain maps). In the v1.4 view a canonical path is
//              translated back through the moves rows the same way the canon
//              v1.4 view does (longest `to` first). A binding that names a
//              command or flag the registry does not hold is an error.

import (
	"fmt"
	"sort"
	"strings"
)

// RootBindingPath is the FlagEnv key of the root command's own flags.
const RootBindingPath = "."

// PathMove is one canon move row: the v1.4 spelling From became the canonical
// path To (both without the leading "nself ", words joined by one space).
type PathMove struct{ From, To string }

// V14Path maps a canonical command path to its v1.4 spelling.
func V14Path(canonical string, moves []PathMove) string {
	best := -1
	for i, m := range moves {
		if canonical == m.To || strings.HasPrefix(canonical, m.To+" ") {
			if best < 0 || len(m.To) > len(moves[best].To) {
				best = i
			}
		}
	}
	if best < 0 {
		return canonical
	}
	return moves[best].From + canonical[len(moves[best].To):]
}

// ApplyFlagEnv sets Flag.env on every flag named in flagEnv. flagEnv is keyed
// by canonical command path (RootBindingPath for the root) then flag name. In
// v1.4 mode (v15 false) the paths are translated through moves first. All
// violations are returned in one error.
func ApplyFlagEnv(reg *Registry, flagEnv map[string]map[string]string, moves []PathMove, v15 bool) error {
	if reg == nil {
		return fmt.Errorf("cmdregistry: ApplyFlagEnv needs a registry")
	}
	paths := make([]string, 0, len(flagEnv))
	for p := range flagEnv {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var problems []string
	for _, canonical := range paths {
		var flags []Flag
		at := canonical
		if canonical == RootBindingPath {
			flags = reg.Root.Flags
			at = "nself (root)"
		} else {
			tree := canonical
			if !v15 {
				tree = V14Path(canonical, moves)
			}
			cmd, ok := reg.Lookup(tree)
			if !ok {
				problems = append(problems, fmt.Sprintf("flag bindings: command %q does not resolve to a command in the tree", tree))
				continue
			}
			flags = cmd.Flags
			at = cmd.Path
		}
		names := make([]string, 0, len(flagEnv[canonical]))
		for n := range flagEnv[canonical] {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			idx := -1
			for i := range flags {
				if flags[i].Name == name {
					idx = i
				}
			}
			if idx < 0 {
				problems = append(problems, fmt.Sprintf("flag bindings: %s has no flag --%s", at, name))
				continue
			}
			flags[idx].Env = strPtr(flagEnv[canonical][name])
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("cmdregistry: %s", strings.Join(problems, "; "))
	}
	return nil
}
