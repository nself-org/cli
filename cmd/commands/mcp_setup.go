package commands

// mcp_setup.go — `nself mcp setup`: the stdio MCP client config and the
// project AGENTS.md (P7-SURF-05, cap:cli.agent-setup).
//
// Purpose: print (default) or write the config an AI client needs to start
//   `nself mcp` for this project, or the AGENTS.md rendered from the registry.
// Inputs: --client claude-code|cursor|codex|generic, --write, --agents-md, --json.
// Outputs: the config or AGENTS.md on stdout; a cwd note or a written-file
//   line; with --json one envelope {client, path, content, written}.
// Constraints: writes only under the project dir; codex and generic are
//   print-only (E401 with --write); an AGENTS.md that is not an unedited
//   generated file is never overwritten; no token is ever written.
// SPORT: CLI-CMD-MCP-SETUP-001

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/agentsetup"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// mcpSetupResult is the envelope data of `mcp setup`.
type mcpSetupResult struct {
	Client  string `json:"client"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Written bool   `json:"written"`
}

var mcpSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Print or write the MCP client config and AGENTS.md for this project",
	Long: `Print the stdio MCP server config for an AI client, or write it into the project.

  claude-code   .mcp.json         (default)
  cursor        .cursor/mcp.json
  codex         prints a TOML block for ~/.codex/config.toml (print-only)
  generic       prints the mcpServers JSON (print-only)

--write creates the file or adds the nself entry to it, keeping every other
entry; a second run changes nothing. --agents-md prints the project AGENTS.md
rendered from the command registry (with --write it writes AGENTS.md unless
the file was edited by hand). The config holds no token: stdio needs none.`,
	Args: cobra.NoArgs,
	RunE: runMCPSetup,
}

func init() {
	f := mcpSetupCmd.Flags()
	f.String("client", agentsetup.ClaudeCode, "Client: "+strings.Join(agentsetup.Clients, ", "))
	f.Bool("write", false, "Write the file into the project instead of printing it")
	f.Bool("agents-md", false, "Print (or with --write, write) the project AGENTS.md")
	f.Bool("json", false, "Emit a v1 JSON envelope")
	mcpCmd.AddCommand(mcpSetupCmd)
}

func runMCPSetup(cmd *cobra.Command, _ []string) error {
	client, _ := cmd.Flags().GetString("client")
	write, _ := cmd.Flags().GetBool("write")
	agentsMD, _ := cmd.Flags().GetBool("agents-md")
	asJSON, _ := cmd.Flags().GetBool("json")
	if !agentsetup.Valid(client) {
		return errs.New("E401", fmt.Sprintf("unknown --client %q (use %s)", client, strings.Join(agentsetup.Clients, ", ")))
	}
	if write && !agentsMD && agentsetup.Path(client) == "" {
		return errs.New("E401", client+" is print-only: run without --write and paste the config into the client")
	}
	res := mcpSetupResult{Client: client, Path: agentsetup.Path(client)}
	var dir string
	if write {
		d, err := resolveProjectDir()
		if err != nil {
			return errs.Wrap("E401", "--write needs an nSelf project", err)
		}
		dir = d
	}
	var err error
	if agentsMD {
		err = setupAgentsMD(&res, dir, write)
	} else {
		res.Content = agentsetup.Config(client)
		if write {
			_, res.Written, err = agentsetup.WriteClient(dir, client)
		}
	}
	if err != nil {
		return err
	}
	return emitMCPSetup(cmd, res, asJSON, write)
}

func setupAgentsMD(res *mcpSetupResult, dir string, write bool) error {
	verbs, err := agentsMDVerbs()
	if err != nil {
		return err
	}
	res.Path, res.Content = agentsetup.FileName, agentsetup.Render(verbs)
	if write {
		_, res.Written, err = agentsetup.WriteAgentsMD(dir, verbs)
	}
	return err
}

func emitMCPSetup(cmd *cobra.Command, res mcpSetupResult, asJSON, write bool) error {
	if asJSON {
		return output.EmitData(output.Default(), "mcp setup", res)
	}
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	switch {
	case write && res.Written:
		_, err := fmt.Fprintf(out, "Wrote %s\n", res.Path)
		return err
	case write:
		_, err := fmt.Fprintf(out, "%s left unchanged (already set up, or edited by hand)\n", res.Path)
		return err
	}
	if res.Path != agentsetup.FileName {
		cwd, _ := os.Getwd()
		fmt.Fprintln(errOut, agentsetup.Note(cwd))
	}
	content := res.Content
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	_, err := fmt.Fprint(out, content)
	return err
}

// agentsMDVerbs lists the canon verbs with their registry summaries, in canon
// order. It reads the v1.5 registry of the running binary without plugin
// mounts, so init, `mcp setup --agents-md` and nself.org render the same bytes.
func agentsMDVerbs() ([]agentsetup.Verb, error) {
	reg, err := buildRegistry(true)
	if err != nil {
		return nil, err
	}
	var out []agentsetup.Verb
	for _, name := range reg.Verbs {
		if c, ok := reg.Lookup(name); ok {
			out = append(out, agentsetup.Verb{Name: name, Summary: c.Summary})
		}
	}
	return out, nil
}

// scaffoldAgentsMD is init's hook: write AGENTS.md without a prompt when the
// file is absent or still the unedited generated one. It reports on stderr
// only (init's stdout is a contract) and never fails init. It reads the live
// command tree, so a command that is not under RootCmd (a test clone) skips it.
func scaffoldAgentsMD(cmd *cobra.Command, dir string, quiet bool) {
	if cmd.Root() != RootCmd {
		return
	}
	// Best effort after init's real work is done: a failure here, even a panic,
	// must not fail init.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not write AGENTS.md: %v\n", r)
		}
	}()
	verbs, err := agentsMDVerbs()
	if err == nil {
		var written bool
		if _, written, err = agentsetup.WriteAgentsMD(dir, verbs); err == nil && written && !quiet {
			fmt.Fprintln(os.Stderr, "Wrote AGENTS.md for AI agents (edit freely; nself will not overwrite your changes)")
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write AGENTS.md: %v\n", err)
	}
}
