package commands

// Canon engine: the argv rewrite (EPIC P7-CANON D2, D3).
//
// Purpose: map the spelling the user typed onto the spelling the running tree
// answers to, before cobra resolves anything. v1.5: an old spelling becomes the
// canonical one (moves, shims, retired hubs, break-outs) or fails with E410
// (removed rows). v1.4: a new spelling silently becomes the old one (moves
// only), because the v1.4 tree is unchanged.
//
// Inputs: the arguments after the program name, the generated table, the root
// command (only to learn which root flags take a value and, in v1.4, what the
// new spelling already resolves to).
//
// Outputs: the rewritten arguments and one canonNote per rewrite.
//
// Constraints: the rewrite never guesses. It touches only the leading command
// words, matches whole words, runs one pass, and leaves the arguments alone when
// the command position cannot be located with certainty (an unknown flag before
// the command, a `--` terminator). It reads only the table: no canon.Load.

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// rewriteCanonArgs applies canonTable to args (the arguments after the program
// name) for the compat mode and returns the arguments to run.
func rewriteCanonArgs(args []string, v15 bool) ([]string, []canonNote, error) {
	return rewriteCanonArgsWith(&canonTable, RootCmd, args, v15)
}

// rewriteCanonArgsWith is rewriteCanonArgs over an explicit table and root.
func rewriteCanonArgsWith(t *canonTableT, root *cobra.Command, args []string, v15 bool) ([]string, []canonNote, error) {
	start := commandStart(root, args)
	if start < 0 {
		return args, nil, nil
	}
	end := start
	for end < len(args) && !strings.HasPrefix(args[end], "-") {
		end++
	}
	words := args[start:end]
	if v15 {
		// compat.V15(P7-CANON-21): old spellings run unchanged -> rewritten to the canonical path with one warning (removed rows fail with E410)
		return rewriteOldToNew(t, root, args, start, words)
	}
	// compat.V15(P7-CANON-21): new spellings are unknown -> silently rewritten to the v1.4 path
	return rewriteNewToOld(t, root, args, start, words), nil, nil
}

func rewriteOldToNew(t *canonTableT, root *cobra.Command, args []string, start int, words []string) ([]string, []canonNote, error) {
	var best canonRowT
	kind, found := "", false
	pick := func(k string, rows []canonRowT) {
		for _, r := range rows {
			if hasPrefix(words, r.From) && (!found || len(r.From) > len(best.From)) && oldSpellingResolves(root, k, r, args[start+len(r.From):]) {
				best, kind, found = r, k, true
			}
		}
	}
	pick("move", t.Moves)
	pick("shim", t.Shims)
	pick("retired", t.RetiredHubs)
	pick("breakout", t.Breakouts)
	pick("removed", t.Removed)
	if !found {
		return args, nil, nil
	}
	if kind == "removed" {
		return nil, nil, removedError(best)
	}
	removal := best.RemovalAt
	if removal == "" {
		removal = canonRemovalDefault
	}
	note := canonNote{Kind: kind, Old: strings.Join(best.From, " "), New: strings.Join(best.To, " "), RemovalAt: removal, Plugin: best.Plugin, From: best.From, To: best.To}
	return splice(args, start, len(best.From), best.To), []canonNote{note}, nil
}

func rewriteNewToOld(t *canonTableT, root *cobra.Command, args []string, start int, words []string) []string {
	var best canonRowT
	found := false
	for _, r := range t.Moves {
		if hasPrefix(words, r.To) && (!found || len(r.To) > len(best.To)) {
			best, found = r, true
		}
	}
	if !found || existingMeaningWins(root, words) {
		return args
	}
	return splice(args, start, len(best.To), best.From)
}

// existingMeaningWins is the v1.4 skip rule (EPIC D3, CANON-21 review ruling): a
// new spelling that the v1.4 tree already answers keeps its meaning, so v1.4
// stays byte-identical. It does when the words resolve to a command exactly, or
// to a runnable command that accepts the remaining words as arguments
// (`doctor heal` runs doctor today; `config secrets rotate` prints config help
// today). A new spelling therefore works in v1.4 only below a command the v1.4
// tree does not have yet (a created hub or a new top-level word).
func existingMeaningWins(root *cobra.Command, words []string) bool {
	cmd, rest, err := root.Find(words)
	if err != nil || cmd == nil || cmd == root {
		return false
	}
	if len(rest) == 0 {
		return true
	}
	if !cmd.Runnable() {
		return false
	}
	if cmd.Args == nil {
		return true
	}
	return cmd.Args(cmd, rest) == nil
}

// oldSpellingResolves reports whether the old spelling's full argv is valid for
// the command a shim or retired-hub row names, so the rewrite cannot turn a
// partial or unknown tail into another command (`migrate reset` into `db reset`,
// bare `service stop` into bare `stop`). It is checked in the pre-relocation
// tree. A retired hub is rewritten only when nothing but known root flags follows
// it; a shim only when its own Args validator accepts the positional tail (or the
// tail asks for --help/-h, which only prints usage) and the first tail word is not
// one of its subcommands. Moves, break-outs and removed
// rows carry the same command and need no check.
func oldSpellingResolves(root *cobra.Command, kind string, r canonRowT, tail []string) bool {
	if kind != "shim" && kind != "retired" {
		return true
	}
	node := walkNames(root, r.From)
	if node == nil || node == root {
		return false
	}
	pos, help, ok := positionals(node, tail)
	if !ok {
		return false
	}
	if kind == "retired" {
		return len(pos) == 0
	}
	if len(pos) > 0 && childNamed(node, pos[0]) != nil {
		return false
	}
	// help only prints the target's usage: it is always safe to forward
	return help || node.Args == nil || node.Args(node, pos) == nil
}

// positionals splits tail into its positional words using the flags node answers
// to (its own and every ancestor's persistent flags) and reports whether a help
// flag was present. A flag that is not known or a value flag with no value makes
// the tail unresolvable.
func positionals(node *cobra.Command, tail []string) (pos []string, help, ok bool) {
	find := func(a string) *pflag.Flag {
		for c := node; c != nil; c = c.Parent() {
			sets := []*pflag.FlagSet{c.PersistentFlags()}
			if c == node {
				sets = append(sets, c.Flags())
			}
			for _, fs := range sets {
				if strings.HasPrefix(a, "--") {
					if f := fs.Lookup(a[2:]); f != nil {
						return f
					}
				} else if len(a) == 2 {
					if f := fs.ShorthandLookup(a[1:]); f != nil {
						return f
					}
				}
			}
		}
		return nil
	}
	for i := 0; i < len(tail); i++ {
		a := tail[i]
		switch {
		case a == "--":
			return append(pos, tail[i+1:]...), help, true
		case !strings.HasPrefix(a, "-") || a == "-":
			pos = append(pos, a)
		case a == "--help" || a == "-h":
			help = true
		default:
			name, hasValue := a, strings.Contains(a, "=")
			if hasValue {
				name = strings.SplitN(a, "=", 2)[0]
			}
			f := find(name)
			if f == nil {
				return nil, false, false
			}
			if f.Value.Type() != "bool" && !hasValue {
				i++
				if i >= len(tail) {
					return nil, false, false
				}
			}
		}
	}
	return pos, help, true
}

// splice replaces n words at args[at:] with repl, without touching args.
func splice(args []string, at, n int, repl []string) []string {
	out := make([]string, 0, len(args)-n+len(repl))
	out = append(out, args[:at]...)
	out = append(out, repl...)
	return append(out, args[at+n:]...)
}

// commandStart returns the index of the first command word, or -1 when it
// cannot be located with certainty. Leading flags are skipped using the root
// flag set (a value flag consumes the next word unless written --flag=value);
// an unknown flag, a `--` terminator or the end of args gives -1.
func commandStart(root *cobra.Command, args []string) int {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return -1
		case !strings.HasPrefix(a, "-"):
			return i
		case strings.Contains(a, "="):
			if rootFlag(root, strings.SplitN(a, "=", 2)[0]) == nil {
				return -1
			}
		default:
			f := rootFlag(root, a)
			if f == nil {
				return -1
			}
			if f.Value.Type() != "bool" {
				i++ // the value word
			}
		}
	}
	return -1
}

// rootFlag looks a leading flag (--name or -x) up on the root command. The help
// and version flags cobra adds at execution time count as bool flags.
func rootFlag(root *cobra.Command, a string) *pflag.Flag {
	look := func(fs *pflag.FlagSet) *pflag.Flag {
		if strings.HasPrefix(a, "--") {
			return fs.Lookup(a[2:])
		}
		if len(a) == 2 {
			return fs.ShorthandLookup(a[1:])
		}
		return nil
	}
	if f := look(root.PersistentFlags()); f != nil {
		return f
	}
	if f := look(root.Flags()); f != nil {
		return f
	}
	if a == "--help" || a == "-h" || a == "--version" {
		return &pflag.Flag{Name: strings.TrimLeft(a, "-"), Value: boolValue{}}
	}
	return nil
}

// boolValue is a stand-in bool flag value for the implicit help flags.
type boolValue struct{}

func (boolValue) String() string   { return "false" }
func (boolValue) Set(string) error { return nil }
func (boolValue) Type() string     { return "bool" }
