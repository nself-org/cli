package reconcile

import (
	"fmt"
	"sort"
)

// destructiveReasons lists why a plan is destructive (EPIC D4).
//
// Inputs: a plan with normalised arrays. Outputs: sorted, de-duplicated,
// stably worded reasons, one per cause: a hand-edited artifact changed or
// removed, an orphan-remove or plugin-remove effect, and a stateful container
// recreated by this command. Never nil. A recreate left to the next start is
// not destructive here: the operator has not been asked to apply it yet.
func destructiveReasons(p *Plan) []string {
	seen := map[string]bool{}
	for _, a := range p.Artifacts {
		if !a.HandEdited {
			continue
		}
		switch a.Action {
		case ActionChange:
			seen[fmt.Sprintf("hand-edited file overwritten: %s", a.Path)] = true
		case ActionRemove:
			seen[fmt.Sprintf("hand-edited file removed: %s", a.Path)] = true
		}
	}
	for _, e := range p.Effects {
		switch e.Kind {
		case EffectOrphanRemove:
			seen[fmt.Sprintf("orphan removed: %s", e.Target)] = true
		case EffectPluginRemove:
			seen[fmt.Sprintf("plugin removed: %s", e.Target)] = true
		}
	}
	for _, c := range p.Containers.Items {
		if c.Action == ContainerRecreate && c.Stateful && c.AppliedBy == AppliedThisCommand {
			seen[fmt.Sprintf("stateful container recreated: %s", c.Service)] = true
		}
	}
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// HandEditedPaths returns the paths of artifacts a human changed that this
// plan would overwrite or remove, in plan order. These are the files that need
// --force in any env (EPIC D4). Callers use it to build the refusal text.
func HandEditedPaths(p Plan) []string {
	var out []string
	for _, a := range p.Artifacts {
		if a.HandEdited && (a.Action == ActionChange || a.Action == ActionRemove) {
			out = append(out, a.Path)
		}
	}
	return out
}
