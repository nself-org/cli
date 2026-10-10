package bindings

// render.go — the generated Markdown of the bindings.
//
// Purpose: the table between the markers of
//          .github/wiki/Configuration-Precedence.md, so the page and the
//          declaration cannot drift (repoqa TestBindingsDocIsCurrent).
// Outputs: Markdown ending in a newline; deterministic order.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

const (
	// BeginMarker and EndMarker fence the generated region of the wiki page.
	BeginMarker = "<!-- BEGIN GENERATED: config-bindings (internal/config/bindings; do not hand edit) -->"
	EndMarker   = "<!-- END GENERATED: config-bindings -->"
)

// RenderMarkdown renders the bound-flags table and the exempt-flags table.
func (b Bindings) RenderMarkdown() string {
	var s strings.Builder
	s.WriteString("### Bound flags\n\n| Command | Flag | Key | Default |\n|---|---|---|---|\n")
	for _, r := range b.Flags {
		def := config.DefaultFor(r.Key)
		if def == "" {
			def = "none"
		}
		fmt.Fprintf(&s, "| %s | `--%s` | `%s` | %s |\n", cmdName(r.Command), r.Flag, r.Key, cell(def))
	}
	s.WriteString("\n### Flags that look like a key and are not bound\n\n| Command | Flag | Reason |\n|---|---|---|\n")
	for _, r := range b.Exempt {
		fmt.Fprintf(&s, "| %s | `--%s` | %s |\n", cmdName(r.Command), r.Flag, cell(r.Reason))
	}
	return s.String()
}

func cmdName(command string) string {
	if command == Root {
		return "`nself` (every command)"
	}
	return "`nself " + command + "`"
}

func cell(v string) string { return strings.ReplaceAll(v, "|", "\\|") }
