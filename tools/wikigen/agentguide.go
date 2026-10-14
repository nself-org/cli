package main

// Agent-Guide.md: the AGENTS.md that `nself init` writes, as a wiki page, and
// the plumbing shared by the three machine pages (MCP-Tools.md,
// HTTP-API-Reference.md, Agent-Guide.md).
// Purpose: one renderer. The page is the generated header plus
// agentsetup.Body, the same body init, `mcp setup --agents-md` and the
// marker hash use, so no second template exists. The body holds nothing that
// varies per project (an empty directory named myproject gets the same bytes),
// so the page is the file without its marker line.
// Inputs: the hermetic registry's canon verbs. Outputs: the page text; the
// machine pages are written or compared by writeMachinePages.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// machineSources are the committed inputs of the machine pages, relative to
// the repository root (wikigen runs there).
type machineSources struct{ golden, openapi, contract string }

var repoSources = machineSources{
	golden:   "internal/mcpgen/testdata/tools.golden.json",
	openapi:  ".github/openapi.json",
	contract: "internal/httpapi/contract.md",
}

// writeMachinePages writes (or, under check, compares) the three machine
// pages and returns the stale names under check, or the number written.
func writeMachinePages(dir string, reg *cmdregistry.Registry, src machineSources, check bool) (stale []string, written int, err error) {
	pages := []struct {
		name   string
		render func() (string, error)
	}{
		{"MCP-Tools.md", func() (string, error) { return renderMCPTools(reg, src.golden) }},
		{"HTTP-API-Reference.md", func() (string, error) { return renderHTTPAPI(reg, src) }},
		{"Agent-Guide.md", func() (string, error) { return renderAgentGuide(agentVerbs(reg)), nil }},
	}
	for _, p := range pages {
		content, err := p.render()
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", p.name, err)
		}
		changed, err := writePage(filepath.Join(dir, p.name), content, check)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", p.name, err)
		}
		switch {
		case changed && check:
			stale = append(stale, p.name)
		case changed:
			written++
		}
	}
	return stale, written, nil
}

// writePage writes content unless the file already holds it; under check it
// only reports whether the file differs.
func writePage(path, content string, check bool) (bool, error) {
	if !pageStale(path, content) {
		return false, nil
	}
	if check {
		return true, nil
	}
	return true, os.WriteFile(path, []byte(content), 0o644)
}

// pluginCommandsParagraph is the closing line of both machine reference
// pages. It stays one line so the pages end with it.
func pluginCommandsParagraph(noun, rule string) string {
	return "**Plugin commands.** Installed plugins add " + noun + " by the same `" + rule +
		"` rule. This page is generated from the builtin registry alone, so it never lists them; each plugin's wiki page lists its own in its machine-surface region, for example the [CI plugin page](https://github.com/nself-org/plugins/wiki/CI).\n"
}

// cell makes text safe inside one table cell.
func cell(s string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(s), " "), "|", `\|`)
}
