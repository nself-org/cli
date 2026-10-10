package cmdregistry

import (
	"fmt"
	"sort"

	"github.com/nself-org/cli/internal/canon"
)

// buildCommand derives one Command and reports the tree-dependent problems of
// its canon entry (side_effect, target, flag overrides, JSON conflicts).
func buildCommand(n node, e canon.Entry, rootPath string, byKey map[string]*node, c *canon.File, dataTypes map[string]any, opts BuildOptions) (Command, []string) {
	var p []string
	at := fmt.Sprintf("commands[%q]", n.key)
	cmd := n.cmd

	cn := e.Canon
	if cn == "" && n.depth >= 2 {
		cn = canon.CanonSubcommand
	}
	runnable := cmd.Run != nil || cmd.RunE != nil
	side := e.SideEffect
	if side == "" {
		if runnable {
			p = append(p, at+": side_effect is required for a runnable command")
		}
		side = canon.SideEffectRead
	}
	output := e.Output
	if output == "" {
		output = canon.OutputDocument
	}

	var target *string
	if e.Target != "" {
		t, ok := byKey[e.Target]
		switch {
		case !ok:
			p = append(p, fmt.Sprintf("%s: target %q does not resolve to a command", at, e.Target))
		case c.Commands[e.Target].Canon == canon.CanonShim:
			p = append(p, fmt.Sprintf("%s: target %q is itself a deprecated-shim; it must name a non-shim command", at, e.Target))
		default:
			full := t.cmd.CommandPath()
			target = &full
		}
	}

	jsonKind, schema, jp := resolveJSON(at, n.key, e, dataTypes, opts)
	p = append(p, jp...)

	flags := localFlags(cmd)
	fp := applyOverrides(at, e, side, output, flags)
	p = append(p, fp...)
	args := parseArgs(cmd.Use)
	for _, name := range e.SecretArgs {
		found := false
		for i := range args {
			if args[i].Name == name {
				args[i].Secret = true
				found = true
			}
		}
		if !found {
			p = append(p, fmt.Sprintf("%s secret_args: %q is not a declared arg", at, name))
		}
	}
	confirm, cp := validateConfirm(at, e.Confirm, runnable, cmd, flags)
	p = append(p, cp...)
	surface := e.Surface
	if surface == "" {
		surface = "all"
	}

	codes := e.ExitCodes
	if opts.V15 && len(e.ExitCodesV15) > 0 {
		codes = e.ExitCodesV15
	}
	exit := map[string]string{}
	for k, v := range codes {
		exit[k] = v
	}

	aliases := append([]string{}, cmd.Aliases...)
	sort.Strings(aliases)
	return Command{
		Path:       cmd.CommandPath(),
		Name:       cmd.Name(),
		Parent:     cmd.Parent().CommandPath(),
		Summary:    cmd.Short,
		Hidden:     cmd.Hidden,
		Deprecated: strPtr(cmd.Deprecated),
		Aliases:    aliases,
		Group:      strPtr(cmd.GroupID),
		Runnable:   runnable,
		Args:       args,
		Flags:      flags,
		Canon:      cn,
		Target:     target,
		SideEffect: side,
		Output:     output,
		JSON:       jsonKind,
		DataSchema: schema,
		ExitCodes:  exit,
		Confirm:    confirm,
		Surface:    surface,
	}, p
}

// resolveJSON derives the json value: envelope iff a data type is registered
// (for a V15OnlyEnvelope path, only when opts.V15), else the declared value
// (default none). A registered type for a path declared legacy is an error.
func resolveJSON(at, key string, e canon.Entry, dataTypes map[string]any, opts BuildOptions) (string, *string, []string) {
	_, registered := dataTypes[key]
	var p []string
	if registered && e.JSON == canon.JSONLegacy && !opts.V15OnlyEnvelope[key] {
		p = append(p, at+": a data type is registered for this path but canon says json: legacy; a command is envelope or legacy, not both")
	}
	if registered && (!opts.V15OnlyEnvelope[key] || opts.V15) {
		s := "schemas/commands/" + dashed(key) + ".v1.schema.json"
		return canon.JSONEnvelope, &s, p
	}
	if e.JSON == "" {
		return canon.JSONNone, nil, p
	}
	return e.JSON, nil, p
}

func dashed(key string) string {
	b := []byte(key)
	for i, ch := range b {
		if ch == ' ' {
			b[i] = '-'
		}
	}
	return string(b)
}

// applyOverrides copies each declared flag override onto its registry flag and
// reports overrides that name a missing flag, fail to escalate the class, or
// declare output stream on a command whose output is not document.
func applyOverrides(at string, e canon.Entry, side, output string, flags []Flag) []string {
	var p []string
	names := make([]string, 0, len(e.Flags))
	for name := range e.Flags {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ov := e.Flags[name]
		fat := fmt.Sprintf("%s flag %q", at, name)
		idx := -1
		for i := range flags {
			if flags[i].Name == name {
				idx = i
			}
		}
		if idx < 0 {
			p = append(p, fat+": no such flag on the command")
			continue
		}
		if ov.SideEffect != "" {
			if canon.Rank(ov.SideEffect) <= canon.Rank(side) {
				p = append(p, fmt.Sprintf("%s: side_effect %q must outrank the command's %q (a flag only escalates)", fat, ov.SideEffect, side))
			}
			flags[idx].SideEffect = strPtr(ov.SideEffect)
		}
		if ov.JSON != "" {
			flags[idx].JSON = strPtr(ov.JSON)
		}
		if ov.Output != "" {
			if output != canon.OutputDocument {
				p = append(p, fmt.Sprintf("%s: output %q is only allowed on a command whose output is document (this one is %q)", fat, ov.Output, output))
			}
			flags[idx].Output = strPtr(ov.Output)
		}
		flags[idx].CLIOnly = ov.CLIOnly
		flags[idx].Secret = ov.Secret
	}
	return p
}
