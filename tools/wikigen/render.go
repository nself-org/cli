package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// placeholderMarker flags prose a human still has to write. `-report` counts it
// and the wiki link-audit surfaces it, so a generated skeleton can never be
// mistaken for a finished page.
const placeholderMarker = "<!-- TODO(docs): needs human prose -->"

// renderPage builds the full T03-shaped page for c, carrying over any prose
// already written for it.
func renderPage(c *cobra.Command, prose proseBlocks) string {
	name := c.Name()
	var b strings.Builder

	fmt.Fprintf(&b, "# nself %s\n\n", name)

	// The summary line is prose, not generated output: hand-written pages often
	// say more than cobra's Short string, and regenerating over the top of that
	// would flatten 90 pages to terminal-help quality.
	// The stored block already contains the "> " blockquote marker, so strip any
	// leading markers before re-adding exactly one. Without this the prefix
	// accumulated on every regeneration ("> > > > Remove Docker resources...").
	summary := strings.TrimSpace(prose.summary)
	for strings.HasPrefix(summary, ">") {
		summary = strings.TrimSpace(strings.TrimPrefix(summary, ">"))
	}
	if summary == "" {
		summary = brand(strings.TrimRight(oneLine(c), ".")) + "."
	}
	b.WriteString(beginProse("summary") + "\n")
	b.WriteString("> " + summary + "\n")
	b.WriteString(endProse("summary") + "\n\n")

	b.WriteString("## Synopsis\n\n```\n")
	fmt.Fprintf(&b, "%s\n", synopsis(c))
	b.WriteString("```\n\n")

	// Aliases are part of the invocation contract (`nself up` for start), so
	// they belong next to the synopsis rather than being left to prose.
	if len(c.Aliases) > 0 {
		quoted := make([]string, 0, len(c.Aliases))
		for _, a := range c.Aliases {
			quoted = append(quoted, "`nself "+a+"`")
		}
		fmt.Fprintf(&b, "**Alias:** %s\n\n", strings.Join(quoted, ", "))
	}

	b.WriteString("## Description\n\n")
	writeProse(&b, "description", prose.description, defaultDescription(c))

	b.WriteString("## Flags\n\n")
	b.WriteString(beginGenerated("flags") + "\n")
	b.WriteString(flagTable(c))
	b.WriteString(endGenerated("flags") + "\n\n")

	if subs := visibleSubcommands(c); len(subs) > 0 {
		b.WriteString("## Subcommands\n\n")
		b.WriteString(beginGenerated("subcommands") + "\n")
		b.WriteString(subcommandTable(subs))
		b.WriteString(endGenerated("subcommands") + "\n\n")
	}

	b.WriteString("## Examples\n\n")
	writeProse(&b, "examples", prose.examples, defaultExamples(c))

	b.WriteString("## See Also\n\n")
	writeProse(&b, "see-also", prose.seeAlso, defaultSeeAlso())

	b.WriteString("← [[Commands]] | [[Home]] →\n")
	return b.String()
}

func beginGenerated(section string) string { return "<!-- BEGIN GENERATED:" + section + " -->" }
func endGenerated(section string) string   { return "<!-- END GENERATED:" + section + " -->" }
func beginProse(section string) string     { return "<!-- BEGIN PROSE:" + section + " -->" }
func endProse(section string) string       { return "<!-- END PROSE:" + section + " -->" }

// writeProse emits a prose block, using the carried-over content when present
// and the generated fallback otherwise.
func writeProse(b *strings.Builder, section, carried, fallback string) {
	body := strings.TrimSpace(carried)
	if body == "" {
		body = strings.TrimSpace(fallback)
	}
	b.WriteString(beginProse(section) + "\n")
	b.WriteString(body + "\n")
	b.WriteString(endProse(section) + "\n\n")
}

func hasPlaceholder(page string) bool { return strings.Contains(page, placeholderMarker) }

// oneLine returns the command's short description, falling back to the first
// line of Long when Short is empty.
func oneLine(c *cobra.Command) string {
	if s := strings.TrimSpace(c.Short); s != "" {
		return s
	}
	for _, line := range strings.Split(c.Long, "\n") {
		if l := strings.TrimSpace(line); l != "" {
			return l
		}
	}
	return "Run `nself " + c.Name() + "`"
}

// synopsis renders the canonical invocation, including positional args when
// cobra's Use string declares them.
func synopsis(c *cobra.Command) string {
	use := strings.TrimSpace(c.Use)
	if use == "" {
		use = c.Name()
	}
	if len(visibleSubcommands(c)) > 0 && !strings.Contains(use, "<") {
		return "nself " + use + " <subcommand> [flags]"
	}
	if strings.Contains(use, " ") {
		return "nself " + use + " [flags]"
	}
	return "nself " + use + " [flags]"
}

// defaultDescription seeds the description from cobra's Long text, which is
// often already a real explanation. Only when there is none does the page get
// a placeholder.
func defaultDescription(c *cobra.Command) string {
	long := strings.TrimSpace(c.Long)
	if long == "" {
		return placeholderMarker + "\n\n" + capitalise(oneLine(c)) + "."
	}
	// Drop a trailing "Examples:" block — it belongs in the Examples section.
	if idx := strings.Index(long, "\nExamples:"); idx >= 0 {
		long = strings.TrimSpace(long[:idx])
	}
	return long
}

// defaultExamples lifts an Examples: block out of cobra's Long text when the
// command already carries one, so 60-odd pages start with real examples rather
// than invented ones.
func defaultExamples(c *cobra.Command) string {
	long := c.Long
	idx := strings.Index(long, "\nExamples:")
	if idx < 0 {
		if ex := strings.TrimSpace(c.Example); ex != "" {
			return "```bash\n" + strings.TrimSpace(dedent(ex)) + "\n```"
		}
		return placeholderMarker + "\n\n```bash\nnself " + c.Name() + "\n```"
	}
	block := strings.TrimSpace(long[idx+len("\nExamples:"):])
	if block == "" {
		return placeholderMarker + "\n\n```bash\nnself " + c.Name() + "\n```"
	}
	return "```bash\n" + strings.TrimSpace(dedent(block)) + "\n```"
}

func defaultSeeAlso() string {
	return "- [[Commands]] — full command index\n- [[Core-Services]] — what a stack is made of"
}

// dedent removes the common leading indentation from a block of text.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	indent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	if indent <= 0 {
		return s
	}
	for i, l := range lines {
		if len(l) >= indent {
			lines[i] = l[indent:]
		}
	}
	return strings.Join(lines, "\n")
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// flagTable renders every local flag with its default, plus the mandatory
// --help row required by template T03.
func flagTable(c *cobra.Command) string {
	type row struct{ name, def, desc string }
	var rows []row

	c.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		name := "`--" + f.Name + "`"
		if f.Shorthand != "" {
			name = "`--" + f.Name + "`, `-" + f.Shorthand + "`"
		}
		def := f.DefValue
		switch def {
		case "":
			def = `""`
		case "[]":
			def = "—"
		}
		rows = append(rows, row{name, "`" + def + "`", escapeCell(f.Usage)})
	})
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	var b strings.Builder
	b.WriteString("| Flag | Default | Description |\n")
	b.WriteString("|------|---------|-------------|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", r.name, r.def, r.desc)
	}
	b.WriteString("| `--help`, `-h` | — | Show help |\n")
	return b.String()
}

func visibleSubcommands(c *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, s := range c.Commands() {
		if s.Hidden || s.Name() == "help" {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func subcommandTable(subs []*cobra.Command) string {
	var b strings.Builder
	b.WriteString("| Name | Description |\n")
	b.WriteString("|------|-------------|\n")
	for _, s := range subs {
		fmt.Fprintf(&b, "| `%s` | %s |\n", s.Name(), escapeCell(brand(oneLine(s))))
	}
	return b.String()
}

var bareURLRe = regexp.MustCompile(`https?://[^\s|<>\)\]]+`)

func escapeCell(s string) string {
	s = bareURLRe.ReplaceAllStringFunc(s, func(m string) string {
		if m[len(m)-1] == '.' || m[len(m)-1] == ',' {
			return "<" + m[:len(m)-1] + ">" + string(m[len(m)-1])
		}
		return "<" + m + ">"
	})
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.ReplaceAll(s, "\n", " ")
}

// brand rewrites the product name for display copy. Per the wiki brand rule
// (SPORT F15) display names use the eta form while system names — the binary,
// paths, commands — stay plain. Cobra help strings are written for a terminal
// and use the plain form throughout, so the published summary line would
// otherwise regress the branding on every page.
//
// Only the capitalised product name is touched; `nself` the binary is left
// alone, as is anything inside a code span.
func brand(s string) string {
	if strings.Contains(s, "`") {
		parts := strings.Split(s, "`")
		for i := 0; i < len(parts); i += 2 { // even indices are outside code spans
			parts[i] = strings.ReplaceAll(parts[i], "nSelf", "ɳSelf")
		}
		return strings.Join(parts, "`")
	}
	return strings.ReplaceAll(s, "nSelf", "ɳSelf")
}
