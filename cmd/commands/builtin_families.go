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
// Outputs:     builtinFamilies (empty until P7-CANON-19 adds admin) and the
//
//	mountBuiltin test seam.
//
// Constraints: builtin families keep root's PersistentPreRunE (D20); a
//
//	disabled family is not mounted and `nself <slug>` exits 1 with
//	E407 (P7-CANON-19 wires the exit path).

import (
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

// builtinFamilies lists the builtin families. Empty here: P7-CANON-19 adds
// admin. Generators mount builtin families unconditionally (minus disable
// markers) so the committed registry contains admin and no installed plugin.
var builtinFamilies []builtinFamily

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
		node := f.build()
		if node == nil {
			continue
		}
		annotateBuiltin(node, f.slug)
		if node.GroupID == "" {
			node.GroupID = groupPlugins
		}
		ensurePluginsGroup(root)
		root.AddCommand(node)
	}
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
