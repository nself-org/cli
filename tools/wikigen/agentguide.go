package main

// Agent-Guide.md: the AGENTS.md that `nself init` writes, as a wiki page.
// Purpose: one renderer. The page is the generated header plus
// agentsetup.Body, the same body init, `mcp setup --agents-md` and the
// marker hash use, so no second template exists. The body holds nothing that
// varies per project (an empty directory named myproject gets the same bytes),
// so the page is the file without its marker line.
// Inputs: the hermetic registry's canon verbs. Outputs: the page text.

import (
	"github.com/nself-org/cli/internal/agentsetup"
	"github.com/nself-org/cli/internal/cmdregistry"
)

// agentVerbs lists the canon verbs with their registry summaries in canon
// order, as `nself init` does (cmd/commands agentsMDVerbs).
func agentVerbs(reg *cmdregistry.Registry) []agentsetup.Verb {
	var out []agentsetup.Verb
	for _, name := range reg.Verbs {
		if c, ok := reg.Lookup(name); ok {
			out = append(out, agentsetup.Verb{Name: name, Summary: c.Summary})
		}
	}
	return out
}

func renderAgentGuide(verbs []agentsetup.Verb) string {
	return generatedNotice + "\n" + agentsetup.Body(verbs)
}
