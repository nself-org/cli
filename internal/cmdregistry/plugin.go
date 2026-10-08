package cmdregistry

// Plugin-mounted nodes in the registry (contract:cli.plugin-command-mount v1).
//
// Purpose:     list commands mounted from plugins — installed ones with
//
//	attributes from the mount annotations, builtin families with
//	attributes from their canon entry — without importing the mount.
//
// Inputs:      the node's cobra annotations, read as plain strings
//
//	("nself.plugin", "nself.mount.source", …). This package must not
//	import internal/plugin; the key literals are the shared contract.
//
// Outputs:     the additive Command.plugin field and Counts.plugin.
//
// Constraints: installed-source nodes skip the canon completeness check (a
//
//	plugin needs no canon.yaml entry); a builtin family's entry must
//	say canon plugin; canon plugin on a non-annotated node stays a
//	validation problem (the file-level rejection is only lifted for
//	annotated nodes, which the file validator cannot see).

import (
	"encoding/json"
	"fmt"

	"github.com/nself-org/cli/internal/canon"
	"github.com/spf13/cobra"
)

// Mount annotation keys (mirrors internal/plugin/mount; plain strings by
// design — no import edge).
const (
	annPluginKey    = "nself.plugin"
	annSourceKey    = "nself.mount.source"
	annSideKey      = "nself.side_effect"
	annOutputKey    = "nself.output"
	annJSONKey      = "nself.json"
	annArgsKey      = "nself.args"
	annFlagsKey     = "nself.flags"
	sourceInstalled = "installed"
	sourceBuiltin   = "builtin"
)

// mountSlug returns the plugin slug of a plugin-mounted node, "" otherwise.
func mountSlug(cmd *cobra.Command) string {
	return cmd.Annotations[annPluginKey]
}

// isInstalledMount reports whether the node was mounted from an installed
// plugin (attributes come from the mount annotations).
func isInstalledMount(cmd *cobra.Command) bool {
	return cmd.Annotations[annPluginKey] != "" && cmd.Annotations[annSourceKey] == sourceInstalled
}

// isBuiltinMount reports whether the node is a builtin plugin family (its
// attributes come from the canon entry, which must say canon plugin).
func isBuiltinMount(cmd *cobra.Command) bool {
	return cmd.Annotations[annPluginKey] != "" && cmd.Annotations[annSourceKey] == sourceBuiltin
}

// buildInstalledCommand derives one installed-plugin Command from the mount
// annotations: canon plugin, side_effect destructive / output document /
// json none by default (contract v1). The args/flags annotations are for the
// surface audit (P7-SURF-30), not the registry shapes.
func buildInstalledCommand(cmd *cobra.Command) Command {
	a := cmd.Annotations
	slug := a[annPluginKey]
	ann := func(key, def string) string {
		if v := a[key]; v != "" {
			return v
		}
		return def
	}
	return Command{
		Path:       cmd.CommandPath(),
		Name:       cmd.Name(),
		Parent:     cmd.Parent().CommandPath(),
		Summary:    cmd.Short,
		Hidden:     cmd.Hidden,
		Deprecated: strPtr(cmd.Deprecated),
		Aliases:    []string{},
		Group:      strPtr(cmd.GroupID),
		Runnable:   cmd.Run != nil || cmd.RunE != nil,
		Args:       pluginArgs(a[annArgsKey]),
		Flags:      pluginFlags(a[annFlagsKey]),
		Canon:      canon.CanonPlugin,
		Plugin:     &slug,
		SideEffect: ann(annSideKey, canon.SideEffectDestructive),
		Output:     ann(annOutputKey, canon.OutputDocument),
		JSON:       ann(annJSONKey, canon.JSONNone),
		ExitCodes:  map[string]string{},
	}
}

func pluginArgs(raw string) []Arg {
	out := []Arg{}
	if raw == "" {
		return out
	}
	var declared []struct {
		Name     string `json:"name"`
		Required bool   `json:"required"`
		Variadic bool   `json:"variadic"`
	}
	if json.Unmarshal([]byte(raw), &declared) != nil {
		return out
	}
	for _, a := range declared {
		out = append(out, Arg{Name: a.Name, Required: a.Required, Variadic: a.Variadic})
	}
	return out
}

func pluginFlags(raw string) []Flag {
	out := []Flag{}
	if raw == "" {
		return out
	}
	var declared []struct {
		Name       string `json:"name"`
		Shorthand  string `json:"shorthand"`
		Type       string `json:"type"`
		Default    string `json:"default"`
		Usage      string `json:"usage"`
		Env        string `json:"env"`
		SideEffect string `json:"side_effect"`
		JSON       string `json:"json"`
		Output     string `json:"output"`
		Hidden     bool   `json:"hidden"`
		Required   bool   `json:"required"`
		Persistent bool   `json:"persistent"`
	}
	if json.Unmarshal([]byte(raw), &declared) != nil {
		return out
	}
	for _, f := range declared {
		out = append(out, Flag{Name: f.Name, Shorthand: strPtr(f.Shorthand), Type: f.Type, Default: f.Default,
			Usage: f.Usage, Hidden: f.Hidden, Required: f.Required, Persistent: f.Persistent,
			Env: strPtr(f.Env), SideEffect: strPtr(f.SideEffect), JSON: strPtr(f.JSON), Output: strPtr(f.Output)})
	}
	return out
}

// pluginReservation is the exact file-level problem canon.Validate reports
// for a canon plugin entry; the file validator cannot see mount annotations,
// so Build lifts it per annotated node.
func pluginReservation(key string) string {
	return fmt.Sprintf("commands[%q]: canon plugin is reserved for plugin-mounted commands and is rejected on cobra-native commands", key)
}

// liftPluginReservation removes the canon-plugin reservation problem for keys
// that carry a mount annotation (builtin families legitimately declare
// canon plugin). Non-annotated keys keep the problem — canon plugin on a
// cobra-native command stays a validation error.
func liftPluginReservation(problems []string, annotated map[string]bool) []string {
	if len(annotated) == 0 {
		return problems
	}
	drop := make(map[string]bool, len(annotated))
	for key := range annotated {
		drop[pluginReservation(key)] = true
	}
	var out []string
	for _, p := range problems {
		if !drop[p] {
			out = append(out, p)
		}
	}
	return out
}
