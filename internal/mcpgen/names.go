package mcpgen

// Purpose: the safety rules between the registry and the tool list: which
// command names may become a tool, in which order commands claim a name, and
// how registry text (summaries, flag usages, defaults) is cleaned before it
// reaches a tool description or an input schema.
// Inputs: registry commands, tool names, free text from a plugin manifest.
// Outputs: yes/no answers, an iteration order and cleaned bounded text.
// Constraints: the name rule is the rule of cmdregistry.ValidateToolNames
// (^[a-z0-9_]+$, at most 64 characters), which is inside the MCP name pattern
// ^[a-zA-Z0-9_-]{1,64}$. A plugin path word must also match the plugin name
// pattern of the mount (^[a-z][a-z0-9-]*$), so a word such as "--help" or "a__b"
// never becomes a tool even though ToolName would turn it into a valid
// identifier. Text is cleaned to bounded single-line plain text: controls,
// format characters (bidi overrides, zero-width, BOM), line and paragraph
// separators and invalid UTF-8 are dropped and white space is collapsed. The
// text of a prompt injection stays text; nothing here claims to detect it.

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
)

const (
	// maxToolName is the longest tool name (the MCP limit).
	maxToolName = 64
	// maxSummaryRunes bounds the registry summary inside a tool description;
	// the longest real summary is far shorter.
	maxSummaryRunes = 500
	// maxSchemaTextRunes bounds a description or a string default of a schema.
	maxSchemaTextRunes = 500
)

var (
	toolNameRe      = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
	pluginSegmentRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// ValidToolName reports whether name may be a tool name.
func ValidToolName(name string) bool { return toolNameRe.MatchString(name) }

// PluginPathOK reports whether every word of a plugin command path (without
// the leading "nself ") is a valid plugin command name.
func PluginPathOK(path string) bool {
	words := strings.Fields(barePath(path))
	if len(words) == 0 {
		return false
	}
	for _, w := range words {
		if !pluginSegmentRe.MatchString(w) {
			return false
		}
	}
	return true
}

// nameProblem says why cmd cannot be a tool named name, or "".
func nameProblem(cmd *cmdregistry.Command, name string) string {
	switch {
	case len(name) > maxToolName:
		return "tool name is longer than 64 characters"
	case !ValidToolName(name):
		return "tool name is not made of a-z, 0-9 and _"
	case cmd.Canon == canon.CanonPlugin && !PluginPathOK(cmd.Path):
		return "a word of the command path is not a valid plugin command name"
	}
	return ""
}

// claimOrder lists the registry indexes with core and subcommand entries
// first, so a plugin command can never take the name of a core command (it is
// skipped with a warning instead, and the core command never fails to load).
func claimOrder(reg *cmdregistry.Registry) []int {
	order := make([]int, 0, len(reg.Commands))
	for i := range reg.Commands {
		if reg.Commands[i].Canon != canon.CanonPlugin {
			order = append(order, i)
		}
	}
	for i := range reg.Commands {
		if reg.Commands[i].Canon == canon.CanonPlugin {
			order = append(order, i)
		}
	}
	return order
}

// cleanText returns s as bounded single-line plain text of at most max runes.
func cleanText(s string, max int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case r == utf8.RuneError:
			continue
		case unicode.IsSpace(r): // includes NEL, NBSP, U+2028 and U+2029
			space = true
			continue
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r):
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	out := b.String()
	if utf8.RuneCountInString(out) <= max {
		return out
	}
	r := []rune(out)
	return strings.TrimRight(string(r[:max-3]), " ") + "..."
}

// cleanSchema cleans, in place, every description and string default of a
// JSON Schema document built from maps and slices.
func cleanSchema(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if s, ok := e.(string); ok && (k == "description" || k == "default") {
				x[k] = cleanText(s, maxSchemaTextRunes)
				continue
			}
			cleanSchema(e)
		}
	case []any:
		for _, e := range x {
			cleanSchema(e)
		}
	}
}
