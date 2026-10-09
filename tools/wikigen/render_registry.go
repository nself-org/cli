package main

// Generated command facts for wiki pages.
// Purpose: render invocation and machine contract from cmdregistry alone.
// Inputs: v1.5 registry commands indexed by full path.
// Outputs: deterministic Markdown sections.
// Constraints: preserve authored PROSE in renderPage, never infer facts from Cobra.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/cmdregistry"
)

func registrySynopsis(c cmdregistry.Command, entries map[string]cmdregistry.Command) string {
	var parts []string
	for _, arg := range c.Args {
		name := arg.Name
		if arg.Variadic {
			name += "..."
		}
		if arg.Required {
			parts = append(parts, "<"+name+">")
		} else {
			parts = append(parts, "["+name+"]")
		}
	}
	if len(parts) == 0 && len(registrySubcommands(c, entries)) > 0 {
		parts = append(parts, "<subcommand>")
	}
	parts = append(parts, "[flags]")
	return c.Path + " " + strings.Join(parts, " ")
}

func registryFlagTable(flags []cmdregistry.Flag) string {
	rows := append([]cmdregistry.Flag(nil), flags...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	var b strings.Builder
	b.WriteString("| Name | Shorthand | Type | Default | Env | Escalates to | JSON override | Description |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, f := range rows {
		if f.Hidden || f.Name == "help" {
			continue
		}
		shorthand := ""
		if f.Shorthand != nil {
			shorthand = "`-" + *f.Shorthand + "`"
		}
		def := f.Default
		if def == "" {
			def = `""`
		}
		fmt.Fprintf(&b, "| `--%s` | %s | `%s` | `%s` | %s | %s | %s | %s |\n",
			f.Name, shorthand, escapeCell(f.Type), escapeCell(def), optionalValue(f.Env),
			optionalValue(f.SideEffect), optionalValue(f.JSON), escapeCell(f.Usage))
	}
	b.WriteString("| `--help` | `-h` | `bool` | `false` | | | | Show help |\n")
	return b.String()
}

func registrySubcommands(parent cmdregistry.Command, entries map[string]cmdregistry.Command) []cmdregistry.Command {
	var out []cmdregistry.Command
	for _, entry := range entries {
		if entry.Parent == parent.Path && !entry.Hidden {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func registrySubcommandTable(subs []cmdregistry.Command) string {
	var b strings.Builder
	b.WriteString("| Name | Description |\n|---|---|\n")
	for _, sub := range subs {
		fmt.Fprintf(&b, "| `%s` | %s |\n", sub.Name, escapeCell(brand(sub.Summary)))
	}
	return b.String()
}

func registryContract(c cmdregistry.Command) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- **Side effect:** `%s`\n", c.SideEffect)
	fmt.Fprintf(&b, "- **Output:** `%s`\n", c.Output)
	fmt.Fprintf(&b, "- **JSON:** `%s`", c.JSON)
	if c.DataSchema != nil {
		fmt.Fprintf(&b, " ([data schema](https://github.com/nself-org/cli/blob/main/%s))", *c.DataSchema)
	}
	b.WriteString("\n")
	if len(c.ExitCodes) == 0 {
		b.WriteString("- **Exit codes:** none declared\n")
		return b.String()
	}
	keys := make([]string, 0, len(c.ExitCodes))
	for state := range c.ExitCodes {
		keys = append(keys, state)
	}
	sort.Strings(keys)
	b.WriteString("- **Exit codes:** ")
	for i, state := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "`%s`: `%s`", state, c.ExitCodes[state])
	}
	b.WriteString("\n")
	return b.String()
}

func optionalValue(value *string) string {
	if value == nil {
		return ""
	}
	return "`" + escapeCell(*value) + "`"
}
