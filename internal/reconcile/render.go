package reconcile

import (
	"fmt"
	"io"
	"strings"
)

// idPrefixLen is how many plan_id characters the human header shows.
const idPrefixLen = 12

// RenderHuman writes the deterministic human form of a plan.
//
// Inputs: a writer and a (normally finalised) Plan. Outputs: a header
// `Plan <first 12 of plan_id> (<command>, env <env>/<class>)`, then one line
// per artifact, effect and container item under Artifacts, Effects and
// Containers headings (a heading is omitted when empty), or `(no changes)`
// for an empty plan. A destructive plan lists its reasons, and a plan that
// needs confirmation says so. The output depends only on the plan's values:
// no time, no map order, no terminal width. Unified diffs are not part of
// it (`--diff` prints those). The error is the writer's.
func RenderHuman(w io.Writer, p Plan) error {
	var sb strings.Builder
	id := p.PlanID
	if len(id) > idPrefixLen {
		id = id[:idPrefixLen]
	}
	fmt.Fprintf(&sb, "Plan %s (%s, env %s/%s)\n", id, p.Command, p.Env, p.EnvClass)
	if p.Empty {
		sb.WriteString("(no changes)\n")
	}
	if len(p.Artifacts) > 0 {
		sb.WriteString("Artifacts:\n")
		for _, a := range p.Artifacts {
			fmt.Fprintf(&sb, "  %-6s %s %s%s\n", a.Action, a.Kind, a.Path, artifactNotes(a))
		}
	}
	if len(p.Effects) > 0 {
		sb.WriteString("Effects:\n")
		for _, e := range p.Effects {
			fmt.Fprintf(&sb, "  %s %s", e.Kind, e.Target)
			if e.Detail != "" {
				fmt.Fprintf(&sb, " (%s)", e.Detail)
			}
			sb.WriteByte('\n')
		}
	}
	if !p.Empty {
		renderContainers(&sb, p.Containers)
	}
	if p.Destructive {
		sb.WriteString("Destructive:\n")
		for _, r := range p.DestructiveReasons {
			fmt.Fprintf(&sb, "  %s\n", r)
		}
	}
	if p.RequiresConfirmation {
		sb.WriteString("Confirmation required: yes\n")
	}
	_, err := io.WriteString(w, sb.String())
	return err
}

// artifactNotes renders the trailing " (3 lines, generated, ...)" text.
func artifactNotes(a Artifact) string {
	var notes []string
	switch {
	case a.DiffLines < 0:
		notes = append(notes, "diff too large")
	case a.DiffLines == 1:
		notes = append(notes, "1 line")
	default:
		notes = append(notes, fmt.Sprintf("%d lines", a.DiffLines))
	}
	if a.Generated {
		notes = append(notes, "generated")
	}
	if a.HandEdited {
		notes = append(notes, "hand-edited")
	}
	if a.Redacted {
		notes = append(notes, "redacted")
	}
	return " (" + strings.Join(notes, ", ") + ")"
}

// renderContainers writes the Containers section: unknown state is stated
// once, items follow one per line. Nothing is written when there is nothing
// to say.
func renderContainers(sb *strings.Builder, c Containers) {
	if c.Known && len(c.Items) == 0 {
		return
	}
	sb.WriteString("Containers:\n")
	if !c.Known {
		sb.WriteString("  state unknown\n")
	}
	for _, it := range c.Items {
		stateful := ""
		if it.Stateful {
			stateful = ", stateful"
		}
		fmt.Fprintf(sb, "  %s %s (%s%s)\n", it.Action, it.Service, it.AppliedBy, stateful)
	}
}
