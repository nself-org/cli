package commands

// config_explain.go — `nself config explain <KEY | command... --flag>`.
//
// Purpose: say where one configuration key comes from: its env name, default,
//          the flags that supply it, the MCP tool and HTTP route parameters
//          that carry those flags, the nself.yaml row, and every source that
//          sets it with the winner. Asked by key (`BASE_DOMAIN`) or by flag
//          (`init --domain`, resolved through the bindings to its key).
// Inputs:  one KEY, or the words of a command followed by one --flag; --reveal
//          and --json.
// Outputs: a table (human) or the v1 envelope (--json). Values are redacted
//          unless --reveal.
// Constraints: flag parsing is off for this command because the target flag
//          (`--domain`) is data, not an option of explain; the arguments are
//          read by hand in config_explain_args.go. The
//          cascade walk is config.ExplainKey, shared with `config env explain`.
//          Read only; never writes.

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/config/bindings"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

var configExplainCmd = &cobra.Command{
	Use:   "explain <key-or-command>... [--flag]",
	Short: "Explain where a configuration key comes from",
	Long: `Explain one configuration key.

  nself config explain BASE_DOMAIN
  nself config explain init --domain

Given a KEY, prints its env name, default, the flags that supply it, the MCP tool
and HTTP route parameter that carry each flag, the nself.yaml row, and every
source that sets it with the winner. Given a command and a flag, resolves the
flag to its key first. A key that is not a known configuration key, or a flag
that supplies no key, exits 1 with E434.

Values are redacted unless --reveal is given. --json prints one envelope.
Precedence is the loader's: a file in the .env cascade replaces a process
environment value, so the process environment wins only when no file sets the key.
ENV is the exception on the way in: its process value chooses the cascade files
before they are read, shown as "cascade selected by".`,
	DisableFlagParsing: true,
	PreRunE:            prepareConfigExplain,
	RunE:               runConfigExplain,
}

func init() {
	configExplainCmd.Flags().Bool("reveal", false, "Show actual values instead of redacting them")
	configCmd.AddCommand(configExplainCmd)
}

func runConfigExplain(cmd *cobra.Command, args []string) error {
	in, err := parseExplainArgs(args)
	if err != nil {
		return err
	}
	if in.help {
		return cmd.Help()
	}
	if len(in.words) == 0 {
		return explainUsage("config explain needs a key or a command and a flag")
	}
	b, err := configBindings()
	if err != nil {
		return err
	}
	reg, v15, err := commandRegistryMode()
	if err != nil {
		return err
	}
	key, from := "", ""
	if in.flag == "" {
		if len(in.words) != 1 {
			return explainUsage("config explain takes one KEY, or a command and a --flag; got %d words and no flag", len(in.words))
		}
		key = in.words[0]
	} else if key, from, err = resolveExplainFlag(reg, b, v15, in.words, in.flag); err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	exp, err := config.ExplainActive(dir, key)
	if err != nil {
		return err
	}
	if !exp.Known {
		return errs.Newf("E434", "%q is not a known configuration key", key)
	}
	if in.reveal && machineInvoked() {
		return refuseRevealForMachine()
	}
	data := buildConfigExplain(exp, b, reg, v15, in.reveal, from)
	if on, _ := cmd.Flags().GetBool("json"); on || in.json {
		return output.EmitData(pilotWriter(), "config explain", data)
	}
	return printConfigExplain(data)
}

// resolveExplainFlag maps "command words + flag" to the key the flag supplies.
// The flag may be declared on the command or inherited from a persistent
// ancestor flag; the binding is looked up on the declaring command.
func resolveExplainFlag(reg *cmdregistry.Registry, b bindings.Bindings, v15 bool, words []string, flag string) (key, from string, err error) {
	path := strings.Join(words, " ")
	from = path + " --" + flag
	cmd, ok := reg.Lookup(path)
	if !ok {
		return "", "", errs.Newf("E434", "nself %s is not a command", path)
	}
	for p := cmd; p != nil; {
		for _, f := range p.Flags {
			if f.Name != flag || (p != cmd && !f.Persistent) {
				continue
			}
			for _, r := range b.Flags {
				if treePath(r.Command, v15) == strings.TrimPrefix(p.Path, "nself ") && r.Flag == flag {
					return r.Key, from, nil
				}
			}
			return "", "", unboundFlagError(b, p.Path, flag, v15)
		}
		next, ok := reg.Lookup(p.Parent)
		if !ok {
			break
		}
		p = next
	}
	if r, ok := registryRootFlag(reg, flag); ok {
		if bound, ok := b.Lookup(bindings.Root, r.Name); ok {
			return bound.Key, from, nil
		}
		return "", "", unboundFlagError(b, "nself", flag, v15)
	}
	return "", "", errs.Newf("E434", "nself %s has no flag --%s", path, flag)
}

func registryRootFlag(reg *cmdregistry.Registry, name string) (cmdregistry.Flag, bool) {
	for _, f := range reg.Root.Flags {
		if f.Name == name {
			return f, true
		}
	}
	return cmdregistry.Flag{}, false
}

// unboundFlagError is E434 for a flag that supplies no key, quoting the
// exemption reason when the flag is deliberately exempt.
func unboundFlagError(b bindings.Bindings, path, flag string, v15 bool) error {
	canonical := strings.TrimPrefix(path, "nself ")
	if path == "nself" {
		canonical = bindings.Root
	}
	for _, e := range b.Exempt {
		if e.Flag == flag && treePath(e.Command, v15) == canonical {
			return errs.Newf("E434", "%s --%s supplies no configuration key: %s", path, flag, e.Reason)
		}
	}
	return errs.Newf("E434", "%s --%s supplies no configuration key (it is not bound in internal/config/bindings)", path, flag)
}

// printConfigExplain renders the human form of the explanation.
func printConfigExplain(d configExplainData) error {
	fmt.Println()
	fmt.Printf("%s\n\n", ui.C(ui.Bold, d.Key))
	if d.ResolvedFrom != nil {
		fmt.Printf("%-12s %s -> %s\n", "Asked as:", *d.ResolvedFrom, d.Key)
	}
	def := d.Default
	if def == "" {
		def = "(none)"
	}
	fmt.Printf("%-12s %s\n%-12s %s\n", "Env name:", d.EnvName, "Default:", def)
	fmt.Printf("%-12s %s (%s cascade)\n", "Environment:", d.Environment, d.CascadeMode)
	if d.Key == "ENV" && d.EnvironmentSource != "" {
		// ENV chooses the cascade before any file is read, so its process
		// value can select the files even when a file wins the final value.
		fmt.Printf("%-12s cascade selected by: %s\n", "", d.EnvironmentSource)
	}
	if len(d.Flags) == 0 {
		fmt.Printf("%-12s none (the key is set by file or environment only)\n", "Flags:")
	}
	for i, f := range d.Flags {
		label := ""
		if i == 0 {
			label = "Flags:"
		}
		line := fmt.Sprintf("%-12s %s --%s", label, f.Command, f.Flag)
		if f.MCPTool != "" {
			line += fmt.Sprintf("   MCP %s %s   HTTP POST %s %s", f.MCPTool, f.MCPParameter, f.HTTPRoute, f.HTTPParameter)
		} else {
			line += "   (cli-only: no MCP or HTTP parameter)"
		}
		fmt.Println(line)
	}
	fmt.Printf("%-12s %s\n\n", "nself.yaml:", d.NselfYAML)
	if len(d.Sources) == 0 {
		fmt.Printf("No file or environment variable sets %s; effective source: %s.\n", d.Key, d.Effective.Source)
		return nil
	}
	tbl := ui.NewTable("Source (lowest first)", "Value", "Winner")
	for _, s := range d.Sources {
		mark := ""
		if s.Winner {
			mark = ui.C(ui.Green, "yes")
		}
		tbl.AddRow(s.Source, explainShown(s.Value), mark)
	}
	tbl.Render()
	fmt.Println()
	ui.Success(fmt.Sprintf("%s wins: %s=%s", d.Effective.Source, d.Key, explainShown(d.Effective.Value)))
	return nil
}

func explainShown(v *string) string {
	if v == nil {
		return "(set, use --reveal to show)"
	}
	return *v
}
