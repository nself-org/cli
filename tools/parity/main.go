// Command parity builds the four-surface parity matrix: one row per top-level
// CLI command, scored against wiki docs, MCP tools, env var docs, and HTTP
// API routes.
//
// Purpose:     CLI-R17, P7-SURF-23. The four surfaces a command can ship on
//
//	(wiki page, MCP tool, documented env vars, HTTP route) drift
//	independently unless something compares them. This generator makes
//	the gap visible and, for the wiki column, blocks CI on it. The MCP
//	and HTTP columns are read from the registry the generators read, so
//	they hold by construction; a new surface-less command shows as "no".
//
// Mode:        documents the v1.5 surface. main sets NSELF_V15=1 in its own
//
//	process (compat.V15 reads the environment on every call), points
//	HOME and the plugin dir at an empty temp dir and calls
//	commands.PrepareTreeForGeneration; the top-level command list is
//	the prepared tree's registry (builtin families only, never an
//	installed plugin, canon "plugin" entries dropped), the same list
//	tools/cmdinventory projects.
//
// Inputs:      the prepared command registry (see Mode),
//
//	commands.MachineRegistry plus mcpgen.Tools (MCP column, see
//	surfaces.go), invoke.Exposure for the HTTP transport (route column),
//	.github/wiki/cmd-<name>.md (wiki column), cmd/commands/<name>*.go +
//	.github/wiki/Config-Env-Vars.md (env column, see envvars.go for the
//	extraction rule).
//
// Outputs:     .github/surface-parity.md (human table) and
//
//	.github/surface-parity.json (machine-readable) by default;
//	-check verifies the committed copies match and writes nothing.
//
// Constraints: read-only against everything except its own two output files.
//
//	Idempotent: running twice with no source changes produces
//	byte-identical output (see internal/repoqa's drift test).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nself-org/cli/cmd/commands"
)

const (
	envVarsWiki    = ".github/wiki/Config-Env-Vars.md"
	wikiDir        = ".github/wiki"
	outMarkdown    = ".github/surface-parity.md"
	outJSON        = ".github/surface-parity.json"
	commandsGlobIn = "cmd/commands"
)

func main() {
	check := flag.Bool("check", false, "verify committed output matches regenerated output; write nothing")
	flag.Parse()
	os.Exit(run(*check))
}

// run does the work behind main so the deferred cleanup of prepareV15 (a temp
// HOME) runs before the process exits.
func run(check bool) int {
	defer prepareV15()()

	rows, err := buildMatrix()
	if err != nil {
		fmt.Fprintf(os.Stderr, "parity: %v\n", err)
		return 1
	}

	md := renderMarkdown(rows)
	js, err := renderJSON(rows)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parity: encode json: %v\n", err)
		return 1
	}

	if check {
		return checkCurrent(md, js)
	}

	if err := os.WriteFile(outMarkdown, []byte(md), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "parity: write %s: %v\n", outMarkdown, err)
		return 1
	}
	if err := os.WriteFile(outJSON, js, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "parity: write %s: %v\n", outJSON, err)
		return 1
	}
	fmt.Printf("wrote %s (%d rows) and %s\n", outMarkdown, len(rows), outJSON)
	return 0
}

// buildMatrix loads the command inventory and scores every top-level command
// against the four surfaces.
func buildMatrix() ([]Row, error) {
	entries, err := liveInventory()
	if err != nil {
		return nil, fmt.Errorf("load inventory: %w", err)
	}

	machine, err := commands.MachineRegistry()
	if err != nil {
		return nil, fmt.Errorf("build machine registry: %w", err)
	}
	tools, err := generatedToolNames(machine)
	if err != nil {
		return nil, err
	}
	surfaces := countSurfaces(machine, tools)

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

		row.MCPTools = surfaces[e.Name].MCPTools
		row.MCPTool = row.MCPTools > 0
		row.HTTPRoutes = surfaces[e.Name].HTTPRoutes
		row.HTTPRoute = row.HTTPRoutes > 0

		vars, err := envVarsForCommand(commandsGlobIn, e.Name)
		if err != nil {
			return nil, fmt.Errorf("scan env vars for %s: %w", e.Name, err)
		}
		row.EnvVars = scoreEnvVars(vars, envDoc)

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
	// MCPTool is true when MCPTools > 0: tools generated for the command or any command below it.
	MCPTool  bool   `json:"mcp_tool"`
	MCPTools int    `json:"mcp_tools"`
	EnvVars  string `json:"env_vars"` // "documented" | "undocumented: VAR, VAR" | "n/a"
	// HTTPRoute is true when HTTPRoutes > 0: OpenAPI routes for the command or any command below it.
	HTTPRoute  bool `json:"http_route"`
	HTTPRoutes int  `json:"http_routes"`
}
