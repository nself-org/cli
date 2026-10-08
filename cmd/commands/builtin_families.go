package commands

// Builtin plugin-command families (contract:cli.plugin-command-mount v1,
// EPIC P7-CANON D19).
//
// Purpose:     the composition root listing the Go packages mounted as plugin
//
//	command families (the admin family, ADR 0022). A family is a real
//	cobra subtree, mounted in every prepareTree call — generators
//	included — unless its disable marker exists.
//
// Inputs:      none — this is registration data.
//
// Outputs:     builtinFamilies (admin) and the
//
//	mountBuiltin test seam.
//
// Constraints: builtin families keep root's PersistentPreRunE (D20); a
//
//	disabled family is not mounted and `nself <slug>` exits 1 with
//	E407 (P7-CANON-19 wires the exit path).

import (
	admin "github.com/nself-org/cli/internal/builtin/admin"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/mount"
	"github.com/spf13/cobra"
)

// builtinFamily is one mountable in-repo command family. build returns a
// fresh subtree; its nodes are annotated nself.plugin=<slug>,
// nself.mount.source=builtin by the mount (plugin_mount.go).
type builtinFamily struct {
	slug  string
	build func() *cobra.Command
}

// builtinFamilies lists the builtin families. Generators mount them (minus disable
// markers) so the committed registry contains admin and no installed plugin.
var builtinFamilies = []builtinFamily{{slug: admin.Slug, build: func() *cobra.Command { return admin.Command(adminDeps()) }}}

func adminDeps() admin.Deps {
	return admin.Deps{
		LoadHealthConfig:  loadHealthConfig,
		OpenBrowserCmd:    openBrowserCmd,
		ResolveEnvFile:    resolveEnvFile,
		SetEnvKeyInFile:   setEnvKeyInFile,
		ShouldOpenBrowser: shouldOpenBrowser,
	}
}

// builtinEnabled reports whether a builtin family is mounted: the plugin
// disable marker (<pluginDir>/<slug>/.disabled) is the shared off switch
// between installed plugins and builtin families (D19).
func builtinEnabled(slug string) bool {
	return !plugin.IsDisabled(slug, resolvePluginDir())
}

// mountBuiltinFamiliesFromList mounts builtin families in runtime and generator
// trees. Every descendant receives the plugin identity; its canon attributes
// remain declared in the fragment rather than copied from the mount.
func mountBuiltinFamiliesFromList(root *cobra.Command) {
	for _, f := range builtinFamilies {
		if !builtinEnabled(f.slug) {
			continue
		}
		alreadyMounted := false
		for _, existing := range root.Commands() {
			if existing.Annotations[mount.AnnSource] == sourceBuiltin && existing.Annotations[mount.AnnPlugin] == f.slug {
				alreadyMounted = true
				break
			}
		}
		if alreadyMounted {
			continue
		}
		node := f.build()
		if node == nil {
			continue
		}
		annotateBuiltin(node, f.slug)
		if node.GroupID == "" {
			// compat.V15(P7-CANON-19): account group -> plugin commands group
			if compat.V15() {
				node.GroupID = groupPlugins
			} else {
				node.GroupID = groupAccount
			}
		}
		ensureBuiltinGroups(root, node)
		root.AddCommand(node)
	}
}

// ensureBuiltinGroups registers every group referenced by a mounted node on
// its parent before Cobra validates groups during Execute.
func ensureBuiltinGroups(root, node *cobra.Command) {
	for _, g := range commandGroups {
		found := false
		for _, current := range root.Groups() {
			if current.ID == g.ID {
				found = true
				break
			}
		}
		if !found {
			root.AddGroup(g)
		}
	}
	if node.GroupID == groupPlugins {
		ensurePluginsGroup(root)
	}
	var visit func(*cobra.Command)
	visit = func(parent *cobra.Command) {
		for _, child := range parent.Commands() {
			if child.GroupID != "" {
				found := false
				for _, g := range parent.Groups() {
					if g.ID == child.GroupID {
						found = true
						break
					}
				}
				if !found {
					parent.AddGroup(&cobra.Group{ID: child.GroupID, Title: child.GroupID + ":"})
				}
			}
			visit(child)
		}
	}
	visit(node)
}

func annotateBuiltin(node *cobra.Command, slug string) {
	if node.Annotations == nil {
		node.Annotations = map[string]string{}
	}
	node.Annotations[mount.AnnPlugin] = slug
	node.Annotations[mount.AnnSource] = sourceBuiltin
	for _, child := range node.Commands() {
		annotateBuiltin(child, slug)
	}
}

// mountBuiltin mounts one family list (test seam over the hook).
func mountBuiltin(root *cobra.Command, families []builtinFamily) {
	saved := builtinFamilies
	builtinFamilies = families
	defer func() { builtinFamilies = saved }()
	mountBuiltinFamiliesFromList(root)
}
