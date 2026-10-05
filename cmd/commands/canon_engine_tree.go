package commands

// Canon engine: the v1.5 tree application (EPIC P7-CANON D2, D3, D10, D13).
//
// Purpose: in v1.5 mode commands live at their canonical path. applyCanon
// relocates moved commands, creates declared hubs, removes shim sources,
// retired hubs, break-outs and removed nodes, leaves a hidden stub at every old
// path (so the registry lists it as a deprecated-shim and a bypassed argv rewrite
// fails with E401 naming the new spelling), hides builtins, rewrites old
// spellings in the prose of relocated subtrees, and gives help-only parents an
// unknown-subcommand validator (D-0062).
//
// Inputs: the generated table (or a fixture table) and a root command.
//
// Outputs: the mutated tree and an undo func that restores it exactly.
//
// Constraints: v1.5 only (v1.4 returns a no-op undo, the tree is untouched); a
// row that cannot be applied (missing source or destination parent, a name
// already taken) is skipped, never overwritten: canonUnresolved lists them and
// the safety tests fail on any.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

// undoList collects the inverse of every mutation, run in reverse.
type undoList []func()

func (u *undoList) add(f func()) { *u = append(*u, f) }
func (u undoList) run() {
	for i := len(u) - 1; i >= 0; i-- {
		u[i]()
	}
}

// applyCanon applies the generated table to root for the compat mode.
func applyCanon(root *cobra.Command, v15 bool) func() {
	return applyCanonWith(&canonTable, root, v15)
}

// applyCanonWith is applyCanon over an explicit table.
func applyCanonWith(t *canonTableT, root *cobra.Command, v15 bool) func() {
	if !v15 {
		return func() {}
	}
	// compat.V15(P7-CANON-21): the cobra tree keeps every command at its v1.4 path -> commands relocated to their canonical path, old paths hold hidden stubs
	var u undoList
	src := resolveRows(t, root)
	helpOnly := helpOnlySet(root)
	for _, h := range sortedByDepth(t.Hubs, func(r canonRowT) int { return len(r.From) }) {
		createHub(&u, root, h)
	}
	for _, r := range sortedByDepth(t.Moves, func(r canonRowT) int { return len(r.To) }) {
		relocate(&u, root, r, src[rowKey("move", r)])
	}
	for _, k := range removalKeys(t) {
		detach(&u, src[k])
	}
	forwards := append(append(append([]canonRowT{}, t.Moves...), t.Shims...), t.RetiredHubs...)
	for _, r := range sortedByDepth(forwards, func(r canonRowT) int { return len(r.From) }) {
		installStub(&u, root, r)
	}
	for _, b := range t.Builtins {
		if c := walkNames(root, b); c != nil {
			hide(&u, c)
		}
	}
	rewriteProse(&u, t, src)
	for c := range helpOnly {
		validate(&u, c)
	}
	return u.run
}

// rowKey names a table row inside the resolved-source map.
func rowKey(kind string, r canonRowT) string { return kind + ":" + strings.Join(r.From, " ") }

// resolveRows finds the node of every row's old path in the untouched tree.
func resolveRows(t *canonTableT, root *cobra.Command) map[string]*cobra.Command {
	m := map[string]*cobra.Command{}
	for k, rows := range map[string][]canonRowT{"move": t.Moves, "shim": t.Shims, "retired": t.RetiredHubs, "breakout": t.Breakouts, "removed": t.Removed} {
		for _, r := range rows {
			if c := walkNames(root, r.From); c != nil && c != root {
				m[rowKey(k, r)] = c
			}
		}
	}
	return m
}

// removalKeys lists the sources removed from the tree: shims, retired hubs,
// break-outs and removed rows (moves are relocated, not removed).
func removalKeys(t *canonTableT) []string {
	var keys []string
	for k, rows := range map[string][]canonRowT{"shim": t.Shims, "retired": t.RetiredHubs, "breakout": t.Breakouts, "removed": t.Removed} {
		for _, r := range rows {
			keys = append(keys, rowKey(k, r))
		}
	}
	sort.Strings(keys)
	return keys
}

func sortedByDepth(rows []canonRowT, depth func(canonRowT) int) []canonRowT {
	out := append([]canonRowT{}, rows...)
	sort.SliceStable(out, func(i, j int) bool { return depth(out[i]) < depth(out[j]) })
	return out
}

// childNamed returns the child of parent with the exact name, or nil.
func childNamed(parent *cobra.Command, name string) *cobra.Command {
	return walkNames(parent, []string{name})
}

func createHub(u *undoList, root *cobra.Command, h canonRowT) {
	parent := walkNames(root, h.From[:len(h.From)-1])
	name := h.From[len(h.From)-1]
	if parent == nil || childNamed(parent, name) != nil {
		return
	}
	hub := &cobra.Command{Use: name, Short: h.Summary, Args: hubUnknownSubcommand, Annotations: map[string]string{annHub: "1"}}
	parent.AddCommand(hub)
	u.add(func() { parent.RemoveCommand(hub) })
}

// relocate moves node to the path r.To: detach, rename the first Use word, drop
// an alias equal to the new name, clear GroupID, attach under the new parent.
func relocate(u *undoList, root *cobra.Command, r canonRowT, node *cobra.Command) {
	if node == nil || len(r.To) == 0 {
		return
	}
	newParent := walkNames(root, r.To[:len(r.To)-1])
	name := r.To[len(r.To)-1]
	if newParent == nil || newParent == node || isDescendant(newParent, node) {
		return
	}
	if existing := childNamed(newParent, name); existing != nil && existing != node {
		return
	}
	oldParent, oldUse, oldAliases, oldGroup, oldAnn := node.Parent(), node.Use, node.Aliases, node.GroupID, node.Annotations
	if oldParent != nil {
		oldParent.RemoveCommand(node)
	}
	if old := node.Name(); old != name {
		node.Use = name + strings.TrimPrefix(node.Use, old)
	}
	node.Aliases = without(node.Aliases, name)
	node.GroupID = ""
	ann := map[string]string{annMovedFrom: strings.Join(r.From, " ")}
	for k, v := range oldAnn {
		ann[k] = v
	}
	node.Annotations = ann
	newParent.AddCommand(node)
	u.add(func() {
		newParent.RemoveCommand(node)
		node.Use, node.Aliases, node.GroupID, node.Annotations = oldUse, oldAliases, oldGroup, oldAnn
		if oldParent != nil {
			oldParent.AddCommand(node)
		}
	})
}

func without(list []string, drop string) []string {
	var out []string
	for _, s := range list {
		if s != drop {
			out = append(out, s)
		}
	}
	if len(out) == len(list) {
		return list
	}
	return out
}

// detach removes node from its current parent.
func detach(u *undoList, node *cobra.Command) {
	if node == nil || node.Parent() == nil {
		return
	}
	parent := node.Parent()
	parent.RemoveCommand(node)
	u.add(func() { parent.AddCommand(node) })
}

func hide(u *undoList, c *cobra.Command) {
	was := c.Hidden
	c.Hidden = true
	u.add(func() { c.Hidden = was })
}

// installStub puts the hidden stub of one forwarded old path in the tree.
func installStub(u *undoList, root *cobra.Command, r canonRowT) {
	parent := walkNames(root, r.From[:len(r.From)-1])
	name := r.From[len(r.From)-1]
	if parent == nil || childNamed(parent, name) != nil {
		return
	}
	from, to := strings.Join(r.From, " "), strings.Join(r.To, " ")
	removal := r.RemovalAt
	if removal == "" {
		removal = canonRemovalDefault
	}
	stub := &cobra.Command{
		Use:                name,
		Short:              "Moved to nself " + to,
		Hidden:             true,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		Deprecated:         fmt.Sprintf("moved to 'nself %s'; the old spelling is removed in %s", to, removal),
		Annotations:        map[string]string{annStub: to},
		RunE: func(*cobra.Command, []string) error {
			return errs.New("E401", fmt.Sprintf("'nself %s' moved to 'nself %s'; run it as 'nself %s'", from, to, to))
		},
	}
	parent.AddCommand(stub)
	u.add(func() { parent.RemoveCommand(stub) })
}

// validate installs the unknown-subcommand validator on a help-only parent.
func validate(u *undoList, c *cobra.Command) {
	if c.Args != nil {
		return
	}
	c.Args = hubUnknownSubcommand
	u.add(func() { c.Args = nil })
}

// canonUnresolved lists the table rows applyCanon cannot apply to root (v1.4
// tree): a missing source, a missing destination parent or a taken name.
func canonUnresolved(t *canonTableT, root *cobra.Command) []string {
	var p []string
	for k, rows := range map[string][]canonRowT{"move": t.Moves, "shim": t.Shims, "retired": t.RetiredHubs, "breakout": t.Breakouts, "removed": t.Removed} {
		for _, r := range rows {
			if walkNames(root, r.From) == nil {
				p = append(p, fmt.Sprintf("%s %q: no such command", k, strings.Join(r.From, " ")))
			}
			if k == "move" && walkNames(root, r.To[:len(r.To)-1]) == nil && !hubPath(t, r.To[:len(r.To)-1]) {
				p = append(p, fmt.Sprintf("move %q: destination parent %q does not exist", strings.Join(r.From, " "), strings.Join(r.To[:len(r.To)-1], " ")))
			}
		}
	}
	sort.Strings(p)
	return p
}

func hubPath(t *canonTableT, words []string) bool {
	for _, h := range t.Hubs {
		if strings.Join(h.From, " ") == strings.Join(words, " ") {
			return true
		}
	}
	return false
}
