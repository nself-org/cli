package commands

// The `help capabilities <query>` command: search what nSelf can do.
//
// Purpose: EPIC P7-SURF D5 self-description. An agent (or a person) asks for a
// task in plain words and gets the commands and installable plugins that fit,
// each with the exact text that runs it. It is a registry command, so it is
// also an MCP tool (nself_help_capabilities) and an HTTP route like any other.
//
// Inputs: the query words, --limit (default 5), the registry of the live tree
// in the current compat mode and the cached plugin catalogue (never fetched).
//
// Outputs: JSON mode writes one v1 envelope (command "help capabilities") with
// data {query, results}; text mode prints one line per hit.
//
// Constraints: scoring is tokenised and case-insensitive. A command scores 3
// per query word found in its path and 1 per word found in its summary; a
// plugin scores 2 per query word found in its slug, summary or tags (category
// and bundles). Words match when equal, when they share a stem prefix
// (diagnose ~ diagnostics) or when a synonym says so (database ~ db). A parent
// command scores two below its best matching subcommand when that is higher,
// so the primary verb is offered next to the specific subcommand. Ties go to
// the shorter path, then the name. Hidden, deprecated, shim, builtin and
// pending commands and non-runnable hubs are not candidates. The search reads
// no network and runs no command.

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/plugin/catalog"
	"github.com/spf13/cobra"
)

const capabilitiesDefaultLimit = 5

var capabilitiesCmd = &cobra.Command{
	Use:   "capabilities <query>...",
	Short: "Search commands and plugins by what you want to do",
	Long: `Search the commands and installable plugins by plain words.

Each hit names the command or plugin and the exact text that runs it. Plugin
hits come from the cached plugin catalogue (run 'nself plugin search' once
while online to fill it).

Examples:
  nself help capabilities rotate a secret
  nself help capabilities kubernetes --limit 3 --json`,
	Args: cobra.MinimumNArgs(1),
	RunE: runCapabilities,
}

func init() {
	capabilitiesCmd.Flags().Int("limit", capabilitiesDefaultLimit, "Maximum number of hits (1 to 50)")
	// help.go registers the help command first (file order); the child joins it.
	if h, _, err := RootCmd.Find([]string{"help"}); err == nil && h != nil && h.Name() == "help" {
		// help takes a command path, so with a child it is an arg-taking parent,
		// not a help-only hub (the canon engine classifies parents by Args).
		h.Args = cobra.ArbitraryArgs
		h.AddCommand(capabilitiesCmd)
	}
}

func runCapabilities(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	if limit < 1 || limit > 50 {
		return errs.Newf("E401", "--limit must be between 1 and 50, got %d", limit)
	}
	reg, err := commandRegistry()
	if err != nil {
		return err
	}
	var plugins []catalog.Entry
	if res, cerr := catalog.LoadCached(catalog.DefaultCacheDir()); cerr == nil {
		plugins = res.Catalog.Plugins
	}
	query := strings.Join(args, " ")
	data := CapabilitiesResult{Query: query, Results: rankCapabilities(reg, plugins, query, limit)}
	if on, jerr := jsonModeOf(cmd); jerr != nil {
		return jerr
	} else if on {
		return output.EmitData(helpWriter(), "help capabilities", data)
	}
	out := cmd.OutOrStdout()
	if len(data.Results) == 0 {
		_, err = fmt.Fprintf(out, "No command or plugin matches %q. Try 'nself help' for the command list.\n", query)
		return err
	}
	for _, m := range data.Results {
		if _, err = fmt.Fprintf(out, "%-34s %s\n", m.Run, m.Summary); err != nil {
			return err
		}
	}
	return nil
}

// stopWords carry no task meaning.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "to": true, "of": true, "for": true, "my": true, "and": true,
	"or": true, "in": true, "on": true, "with": true, "is": true, "it": true, "do": true, "how": true,
	"i": true, "me": true, "we": true, "can": true, "all": true,
}

// words lowercases s and splits it on anything that is not a letter or digit.
func words(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if !stopWords[w] {
			out = append(out, w)
		}
	}
	return out
}

// sameWord reports whether two words are equal or share a stem prefix of at
// least five letters that covers all but two letters of the shorter word.
func sameWord(a, b string) bool {
	if a == b {
		return true
	}
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	short := len(a)
	if len(b) < short {
		short = len(b)
	}
	return n >= 5 && n >= short-2
}

// synonyms maps a query word to other spellings of the same thing; the
// relation is read in both directions.
var synonyms = [][2]string{
	{"database", "db"}, {"kubernetes", "k8s"}, {"run", "up"}, {"start", "up"}, {"stop", "down"},
}

// expand returns w and its synonyms.
func expand(w string) []string {
	out := []string{w}
	for _, p := range synonyms {
		switch w {
		case p[0]:
			out = append(out, p[1])
		case p[1]:
			out = append(out, p[0])
		}
	}
	return out
}

// hits counts the query words that match at least one word of field.
func hits(query, field []string) int {
	n := 0
	for _, q := range query {
	next:
		for _, alt := range expand(q) {
			for _, f := range field {
				if sameWord(alt, f) {
					n++
					break next
				}
			}
		}
	}
	return n
}

// candidate reports whether a registry command can be a hit.
func candidate(c *cmdregistry.Command) bool {
	if c.Hidden || c.Deprecated != nil || !c.Runnable {
		return false
	}
	switch c.Canon {
	case canon.CanonCore, canon.CanonSubcommand, canon.CanonPlugin:
		return true
	}
	return false
}

// rankCapabilities scores the registry commands and the catalogue plugins
// against query and returns the best limit hits.
func rankCapabilities(reg *cmdregistry.Registry, plugins []catalog.Entry, query string, limit int) []CapabilityMatch {
	q := dedupe(words(query))
	out := []CapabilityMatch{}
	if len(q) == 0 {
		return out
	}
	type scored struct {
		m     CapabilityMatch
		depth int
	}
	var all []scored
	byPath := map[string]int{}
	for i := range reg.Commands {
		c := &reg.Commands[i]
		if !candidate(c) {
			continue
		}
		path := strings.TrimPrefix(c.Path, "nself ")
		pw := words(path)
		s := 3*hits(q, pw) + hits(q, words(c.Summary))
		byPath[path] = len(all)
		all = append(all, scored{CapabilityMatch{Kind: "command", Path: path, Summary: c.Summary, Run: "nself " + path, Score: s}, len(pw)})
	}
	// A parent answers for its subcommands: two below the best one when higher.
	raw := make([]int, len(all))
	for i := range all {
		raw[i] = all[i].m.Score
	}
	for i := range all {
		if raw[i] == 0 {
			continue
		}
		parts := strings.Fields(all[i].m.Path)
		for n := len(parts) - 1; n >= 1; n-- {
			if j, ok := byPath[strings.Join(parts[:n], " ")]; ok && raw[i]-2 > all[j].m.Score {
				all[j].m.Score = raw[i] - 2
			}
		}
	}
	kept := all[:0]
	for _, c := range all {
		if c.m.Score > 0 {
			kept = append(kept, c)
		}
	}
	all = kept
	seen := map[string]bool{}
	for _, p := range plugins {
		if !p.Installable || seen[p.Slug] {
			continue
		}
		tags := append(words(p.Category), words(strings.Join(p.Bundles, " "))...)
		fields := append(append(words(p.Slug), words(p.Description)...), tags...)
		if s := 2 * hits(q, fields); s > 0 {
			seen[p.Slug] = true
			all = append(all, scored{CapabilityMatch{Kind: "plugin", Slug: p.Slug, Summary: p.Description, Run: "nself add " + p.Slug, Score: s}, 1})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.m.Score != b.m.Score {
			return a.m.Score > b.m.Score
		}
		if a.depth != b.depth {
			return a.depth < b.depth
		}
		return a.m.Run < b.m.Run
	})
	for i := 0; i < len(all) && i < limit; i++ {
		out = append(out, all[i].m)
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range in {
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}
