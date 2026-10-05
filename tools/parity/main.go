// Command parity builds the four-surface parity matrix: one row per top-level
// CLI command, scored against wiki docs, MCP tool coverage, env var docs, and
// OpenAPI routes.
//
// Purpose:     CLI-R17. The four surfaces a command can ship on (wiki page,
//
//	MCP tool, documented env vars, OpenAPI route) drift independently
//	today — nothing catches a new command that ships without a wiki
//	page, or an MCP tool whose backing command got renamed. This
//	generator makes the gap visible and, for the wiki column, blocks
//	CI on it.
//
// Mode:        documents the v1.5 surface. main sets NSELF_V15=1 in its own
//
//	process (compat.V15 reads the environment on every call) and calls
//	commands.PrepareTreeForGeneration; the top-level command list is
//	the prepared tree's registry (builtin families only, never an
//	installed plugin, canon "plugin" entries dropped), the same list
//	tools/cmdinventory projects.
//
// Inputs:      the prepared command registry (see Mode),
//
//	.github/wiki/cmd-<name>.md (wiki column), cmd/commands/
//	mcp.go + mcp_sentry.go (MCP column, read as text — see mcptools.go
//	for the matching rule), cmd/commands/<name>*.go + .github/wiki/
//	Config-Env-Vars.md (env column, see envvars.go for the extraction
//	rule).
//
// Outputs:     .github/surface-parity.md (human table) and
//
//	.github/surface-parity.json (machine-readable) by default;
//	-check verifies the committed copies match and writes nothing.
//
// Constraints: read-only against everything except its own two output files.
//
//	The OpenAPI Route column is always "n/a": internal/apidocs exists
//	and is wired into `nself build` (internal/build/orchestrator.go),
//	but it documents the generated backend's HTTP surface, not the
//	CLI's own commands — see openapi.go in this package for the full
//	investigation and citations. Idempotent: running twice with no
//	source changes produces byte-identical output (see internal/repoqa's
//	drift test).
package main

import (
	"flag"
	"fmt"
	"os"
)

const (
	mcpMainPath    = "cmd/commands/mcp.go"
	mcpSentryPath  = "cmd/commands/mcp_sentry.go"
	envVarsWiki    = ".github/wiki/Config-Env-Vars.md"
	wikiDir        = ".github/wiki"
	outMarkdown    = ".github/surface-parity.md"
	outJSON        = ".github/surface-parity.json"
	commandsGlobIn = "cmd/commands"
)

func main() {
	check := flag.Bool("check", false, "verify committed output matches regenerated output; write nothing")
	flag.Parse()

	defer prepareV15()()

	rows, err := buildMatrix()
	if err != nil {
		fmt.Fprintf(os.Stderr, "parity: %v\n", err)
		os.Exit(1)
	}

	md := renderMarkdown(rows)
	js, err := renderJSON(rows)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parity: encode json: %v\n", err)
		os.Exit(1)
	}

	if *check {
		os.Exit(checkCurrent(md, js))
	}

	if err := os.WriteFile(outMarkdown, []byte(md), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "parity: write %s: %v\n", outMarkdown, err)
		os.Exit(1)
	}
	if err := os.WriteFile(outJSON, js, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "parity: write %s: %v\n", outJSON, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d rows) and %s\n", outMarkdown, len(rows), outJSON)
}

// buildMatrix loads the command inventory and scores every top-level command
// against the four surfaces.
func buildMatrix() ([]Row, error) {
	entries, err := liveInventory()
	if err != nil {
		return nil, fmt.Errorf("load inventory: %w", err)
	}

	mcpTokens, err := mcpToolTokens(mcpMainPath, mcpSentryPath)
	if err != nil {
		return nil, fmt.Errorf("parse mcp tools: %w", err)
	}

	envDoc, err := os.ReadFile(envVarsWiki)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", envVarsWiki, err)
	}

	rows := make([]Row, 0, len(entries))
	for _, e := range entries {
		row := Row{
			Name:    e.Name,
			Path:    e.Path,
			GroupID: e.GroupID,
		}
		row.WikiPage = hasWikiPage(wikiDir, e.Name)

		row.MCPTool = mcpTokens[e.Name]

		vars, err := envVarsForCommand(commandsGlobIn, e.Name)
		if err != nil {
			return nil, fmt.Errorf("scan env vars for %s: %w", e.Name, err)
		}
		row.EnvVars = scoreEnvVars(vars, envDoc)

		row.OpenAPI = openAPIColumnValue

		rows = append(rows, row)
	}
	return rows, nil
}

// checkCurrent compares freshly rendered output against the committed files
// and reports drift on stderr. Returns the process exit code.
func checkCurrent(md string, js []byte) int {
	code := 0
	committedMD, err := os.ReadFile(outMarkdown)
	if err != nil || string(committedMD) != md {
		fmt.Fprintf(os.Stderr, "parity: %s is stale (run `make parity`)\n", outMarkdown)
		code = 1
	}
	committedJSON, err := os.ReadFile(outJSON)
	if err != nil || string(committedJSON) != string(js) {
		fmt.Fprintf(os.Stderr, "parity: %s is stale (run `make parity`)\n", outJSON)
		code = 1
	}
	return code
}

// Row is one line of the parity matrix.
type Row struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	GroupID  string `json:"group_id,omitempty"`
	WikiPage bool   `json:"wiki_page"`
	MCPTool  bool   `json:"mcp_tool"`
	EnvVars  string `json:"env_vars"` // "documented" | "undocumented: VAR, VAR" | "n/a"
	OpenAPI  string `json:"openapi"`  // always "n/a" — see package doc.
}
