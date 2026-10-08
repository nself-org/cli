package commands

// Plugin-command mount (contract:cli.plugin-command-mount v1, EPIC P7-CANON
// D6/D19/D20/D21).
//
// Purpose:     mount installed plugin commands and builtin families as real
//
//	cobra subtrees, so `nself <command> …` runs the plugin binary and
//	the command shows up in help and the registry.
//
// Inputs:      the cobra root, mount.Specs from internal/plugin/mount, the
//
//	builtin family list, the generated canon table.
//
// Outputs:     mounted cobra nodes carrying nself.* annotations, stderr
//
//	warnings for problems (once per process), the "Plugin Commands:"
//	group.
//
// Constraints: an invocation whose first token is a flag keeps today's
//
//	behaviour (the old intercept never proxied those, so mounting is
//	skipped and they stay byte-identical to the base binary); every
//	mounted node is runnable and DisableFlagParsing, so argv reaches
//	the binary exactly as the unknown-command proxy sent it; installed
//	roots get no-op pre/post-run hooks (D20); no canon.Load anywhere on
//	this path (D15: verbs come from the generated table).

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/mount"
	"github.com/spf13/cobra"
)

const (
	groupPlugins      = "plugins"
	pluginsGroupTitle = "Plugin Commands:"
	// sourceInstalled and sourceBuiltin are the nself.mount.source values.
	sourceInstalled = "installed"
	sourceBuiltin   = "builtin"
)

// alwaysNativeNames are cobra-native command names that may not be in the
// tree yet at mount time (cobra adds help and completion during Execute,
// after the mount) but that a plugin may still never claim.
var alwaysNativeNames = []string{"help", "completion", "man", "version"}

// mountWarned guards the once-per-process problem warnings.
var mountWarned bool

// mountInstalledPluginsFromDisk is the prepareTree hook: discover installed
// plugins and mount them. Skipped when the invocation starts with a flag —
// those invocations never reached the old proxy either (dispatch.go's
// intercept skips them), so their output stays byte-identical to the base
// binary in both modes.
func mountInstalledPluginsFromDisk(root *cobra.Command) {
	if !invocationAllowsMount() {
		return
	}
	specs, problems := mount.Discover(resolvePluginDir(), canonTable.Verbs)
	mountInstalled(root, specs, problems)
}

// invocationAllowsMount reports whether this process's argv may mount: the
// first token after the binary name must not be a flag.
func invocationAllowsMount() bool {
	if len(os.Args) < 2 {
		return true
	}
	return os.Args[1] != "" && os.Args[1][0] != '-'
}

// mountInstalled adds one cobra subtree per spec, warns about the problems
// once per process, and adds the Plugin Commands group. Specs whose command
// clashes with the tree are turned into E405 problems here (Discover cannot
// see the tree).
func mountInstalled(root *cobra.Command, specs []mount.Spec, problems []mount.Problem) {
	mountable, treeProblems := planMount(root, specs)
	problems = append(problems, treeProblems...)
	for _, s := range mountable {
		root.AddCommand(installedNode(s))
	}
	if len(mountable) > 0 {
		ensurePluginsGroup(root)
	}
	warnMountProblems(problems)
}

// planMount partitions specs into mountable ones and problems, against the
// CURRENT tree: a command equal to a cobra-native command name or alias is
// not mounted (E405, both owners named); a command equal to a core command
// the generated breakouts table moves to this same slug is skipped silently
// in v1.4 mode (core wins; core already proxies).
func planMount(root *cobra.Command, specs []mount.Spec) ([]mount.Spec, []mount.Problem) {
	var mountable []mount.Spec
	var problems []mount.Problem
	for _, s := range specs {
		if owner := breakoutOwner(s.Command); owner != "" && owner != s.Slug {
			problems = append(problems, mount.Problem{Code: "E405", Slug: s.Slug,
				Message: fmt.Sprintf("[E405] command %q of plugin %q belongs to breakout plugin %q; not mounted", s.Command, s.Slug, owner)})
			continue
		}
		// compat.V15(P7-CANON-01): same-owner breakout stays core in v1.4 -> mounts from its plugin in v1.5
		if !compat.V15() && silentBreakout(s) {
			continue
		}
		if owner := nativeClash(root, s.Command); owner != "" {
			problems = append(problems, mount.Problem{Code: "E405", Slug: s.Slug,
				Message: fmt.Sprintf("[E405] command %q of plugin %q collides with the core command %q; not mounted", s.Command, s.Slug, owner)})
			continue
		}
		mountable = append(mountable, s)
	}
	return mountable, problems
}

// silentBreakout reports whether the canon table breaks the core command
// with this name out to the same plugin slug (D10): in v1.4 mode core stays
// and the plugin is quietly not mounted.
func silentBreakout(s mount.Spec) bool {
	return breakoutOwner(s.Command) == s.Slug
}

func breakoutOwner(name string) string {
	for _, row := range canonTable.Breakouts {
		if len(row.From) > 0 && row.From[0] == name {
			return row.Plugin
		}
	}
	return ""
}

// nativeClash returns the cobra-native command (name or alias) that already
// owns name, or "". Nodes mounted by a plugin are not native.
func nativeClash(root *cobra.Command, name string) string {
	for _, n := range alwaysNativeNames {
		if n == name {
			return n
		}
	}
	for _, c := range root.Commands() {
		if c.Annotations[mount.AnnSource] == sourceInstalled {
			continue
		}
		if c.Name() == name {
			return c.Name()
		}
		for _, a := range c.Aliases {
			if a == name {
				return c.Name() + " (alias " + a + ")"
			}
		}
	}
	return ""
}

// ensurePluginsGroup registers the Plugin Commands group once.
func ensurePluginsGroup(root *cobra.Command) {
	for _, g := range root.Groups() {
		if g.ID == groupPlugins {
			return
		}
	}
	root.AddGroup(&cobra.Group{ID: groupPlugins, Title: pluginsGroupTitle})
}

// installedNode builds the cobra subtree of one spec: a runnable root and a
// runnable node per subcommand segment, every node DisableFlagParsing with
// the proxy RunE, the root carrying no-op pre/post-run hooks (D20) and the
// GroupID plugins.
func installedNode(s mount.Spec) *cobra.Command {
	root := &cobra.Command{
		Use:                s.Command,
		Short:              s.Summary,
		DisableFlagParsing: true,
		GroupID:            groupPlugins,
		PersistentPreRunE:  func(*cobra.Command, []string) error { return nil },
		PersistentPostRunE: func(*cobra.Command, []string) error { return nil },
	}
	annotate(root, s.Slug, sourceInstalled, s.SideEffect, s.Output, s.JSON, nil, nil, s.Confirm, s.Surface)
	proxyRunE(root, s, nil)
	byPath := map[string]*cobra.Command{"": root}
	for _, sub := range s.Subcommands {
		parent := root
		key := ""
		for _, seg := range sub.Path {
			key += seg
			node, ok := byPath[key]
			if !ok {
				node = &cobra.Command{Use: seg, DisableFlagParsing: true}
				annotate(node, s.Slug, sourceInstalled, "", "", "", nil, nil, nil, "")
				proxyRunE(node, s, strings.Fields(key))
				parent.AddCommand(node)
				byPath[key] = node
			}
			parent, key = node, key+" "
		}
		// The leaf carries the subcommand's attributes; intermediate nodes
		// keep the defaults (destructive/none/document).
		annotate(parent, s.Slug, sourceInstalled, sub.SideEffect, sub.Output, sub.JSON, sub.Args, sub.Flags, sub.Confirm, sub.Surface)
		proxyRunE(parent, s, sub.Path)
	}
	return root
}

// proxyRunE sets the node's RunE: exec the plugin binary with the node's own
// segments below the plugin root plus the remaining args, root persistent
// flags stripped (--json survives; --no-monorepo and friends never reached a
// proxied plugin and still do not).
func proxyRunE(node *cobra.Command, s mount.Spec, segs []string) {
	own := append([]string{}, segs...)
	node.RunE = func(cmd *cobra.Command, args []string) error {
		argv := append(append([]string{}, own...), stripRootPersistentFlags(args)...)
		return plugin.ProxyBinaryAt(s.BinaryPath, resolvePluginDir(), s.Slug, argv)
	}
}

// annotate writes the mount annotations (defaults applied: side_effect
// destructive, output document, json none).
func annotate(node *cobra.Command, slug, source, side, output, jsonKind string, args, flags, confirm []byte, surface string) {
	a := node.Annotations
	if a == nil {
		a = map[string]string{}
		node.Annotations = a
	}
	a[mount.AnnPlugin] = slug
	a[mount.AnnSource] = source
	a[mount.AnnSideEffect] = eff(side, mount.DefaultSideEffect)
	a[mount.AnnOutput] = eff(output, mount.DefaultOutput)
	a[mount.AnnJSON] = eff(jsonKind, mount.DefaultJSON)
	if len(args) > 0 {
		a[mount.AnnArgs] = string(args)
	}
	if len(flags) > 0 {
		a[mount.AnnFlags] = string(flags)
	}
	if len(confirm) > 0 && string(confirm) != "null" {
		a[mount.AnnConfirm] = string(confirm)
	}
	if surface != "" {
		a[mount.AnnSurface] = surface
	}
}

func eff(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// warnMountProblems prints one stderr line per problem, once per process,
// unless warnings are silenced.
func warnMountProblems(problems []mount.Problem) {
	if len(problems) == 0 || mountWarned || warningsSilenced(os.Args) {
		return
	}
	for _, p := range problems {
		message := strings.TrimPrefix(p.Message, "["+p.Code+"] ")
		fmt.Fprintf(os.Stderr, "warning: [%s] %s\n", p.Code, safeMountText(message))
	}
	mountWarned = true
}

func safeMountText(s string) string {
	quoted := strconv.QuoteToASCII(s)
	return quoted[1 : len(quoted)-1]
}
