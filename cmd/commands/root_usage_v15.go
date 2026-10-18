package commands

// Root usage template for v1.5.
//
// Purpose: ADR 0016 promises `nself --help` fits one screen. The v1.4 template
// groups commands under eight concern titles (cobra groups); with 20 verbs the
// titles and blank lines cost more than the commands, and a group cannot be
// removed from a cobra root once added, so a mode switch in one process (the
// machine registry builds the v1.5 tree inside a v1.4 process) must not touch
// groups. The v1.5 root therefore lists its verbs from one template, chosen at
// render time by the compat mode, and keeps the "Plugin Commands" group.
//
// Inputs: the compat mode (v15 template function, read on every render).
//
// Outputs: RootCmd's usage template. Every other command, and the root in
// v1.4, renders cobra's default template byte for byte (ADR 0021).
//
// Constraints: the v1.5 branch applies to the root only (no parent).

import (
	"github.com/nself-org/cli/internal/compat"
	"github.com/spf13/cobra"
)

// usageRootV15 is the v1.5 root body: one Commands list, then the plugin group.
const usageRootV15 = `Usage:
  {{.CommandPath}} [command]{{$cmds := .Commands}}

Commands:{{range $cmds}}{{if (and (ne .GroupID "plugins") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{range $group := .Groups}}{{if eq $group.ID "plugins"}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}

Use "{{.CommandPath}} [command] --help" for more information about a command.
`

func init() {
	cobra.AddTemplateFunc("v15", compat.V15)
	RootCmd.SetUsageTemplate(`{{if (and v15 (not .HasParent))}}` + usageRootV15 + `{{else}}` + RootCmd.UsageTemplate() + `{{end}}`)
}
