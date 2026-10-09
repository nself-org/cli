package cmdregistry

// Purpose: validate surface declarations and derive names and effective values.
// Inputs: canon confirmation, registry commands and the flags set by a caller.
// Outputs: validated confirmations, deterministic tool/route names and classes.
// Constraints: helpers do not mutate their inputs.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/canon"
	"github.com/spf13/cobra"
)

func validateConfirm(at string, src *canon.Confirm, runnable bool, cmd *cobra.Command, declared []Flag) (*Confirm, []string) {
	if src == nil {
		return nil, nil
	}
	var problems []string
	if !runnable {
		problems = append(problems, at+" confirm: command is not runnable")
	}
	seen := map[string]bool{}
	for _, name := range src.Flags {
		if name == "" || seen[name] {
			problems = append(problems, fmt.Sprintf("%s confirm.flags: %q is empty or duplicated", at, name))
		}
		seen[name] = true
	}
	if src.Plan != nil && (src.Plan.Flag == "" || src.Plan.IDFlag == "") {
		problems = append(problems, at+" confirm.plan: flag and id_flag are required")
	}
	flagType := func(name string) string {
		for _, f := range declared {
			if f.Name == name {
				return f.Type
			}
		}
		for c := cmd; c != nil; c = c.Parent() {
			for _, f := range localFlags(c) {
				if f.Name == name {
					return f.Type
				}
			}
		}
		return ""
	}
	check := func(name, typ, path string) {
		got := flagType(name)
		if got == "" {
			problems = append(problems, fmt.Sprintf("%s %s: flag %q does not exist", at, path, name))
		} else if got != typ {
			problems = append(problems, fmt.Sprintf("%s %s: flag %q must be %s, got %s", at, path, name, typ, got))
		}
	}
	for _, name := range src.Flags {
		check(name, "bool", "confirm.flags")
	}
	var plan *PlanForm
	if src.Plan != nil {
		check(src.Plan.Flag, "bool", "confirm.plan.flag")
		check(src.Plan.IDFlag, "string", "confirm.plan.id_flag")
		plan = &PlanForm{Flag: src.Plan.Flag, IDFlag: src.Plan.IDFlag}
	}
	if len(problems) != 0 {
		return nil, problems
	}
	return &Confirm{Flags: append([]string{}, src.Flags...), Plan: plan}, nil
}

func barePath(path string) string { return strings.TrimPrefix(strings.TrimSpace(path), "nself ") }

// ToolName converts a command path to its MCP tool identifier.
func ToolName(path string) string {
	return "nself_" + strings.NewReplacer("-", "_", " ", "_").Replace(barePath(path))
}

// RoutePath converts a command path to its HTTP route.
func RoutePath(path string) string {
	return "/v1/commands/" + strings.ReplaceAll(barePath(path), " ", "/")
}

// EffectiveSideEffect is the highest class produced by set flag overrides.
func EffectiveSideEffect(cmd *Command, set map[string]bool) string {
	if cmd == nil {
		return ""
	}
	best := cmd.SideEffect
	for _, flag := range cmd.Flags {
		if set[flag.Name] && flag.SideEffect != nil && canon.Rank(*flag.SideEffect) > canon.Rank(best) {
			best = *flag.SideEffect
		}
	}
	return best
}

// EffectiveOutput selects stream when a set flag declares a stream override.
func EffectiveOutput(cmd *Command, set map[string]bool) string {
	if cmd == nil {
		return ""
	}
	for _, flag := range cmd.Flags {
		if set[flag.Name] && flag.Output != nil && *flag.Output == canon.OutputStream {
			return canon.OutputStream
		}
	}
	return cmd.Output
}

var toolCharset = regexp.MustCompile(`^[a-z0-9_]+$`)

// ValidateToolNames rejects invalid or colliding generated tool identifiers.
func ValidateToolNames(reg *Registry) error {
	if reg == nil {
		return fmt.Errorf("registry is nil")
	}
	seen := map[string]string{}
	var problems []string
	for _, cmd := range reg.Commands {
		name := ToolName(cmd.Path)
		if len(name) > 64 || !toolCharset.MatchString(name) {
			problems = append(problems, fmt.Sprintf("%s: invalid tool name %q", cmd.Path, name))
		}
		if previous, ok := seen[name]; ok {
			problems = append(problems, fmt.Sprintf("%s and %s: tool name collision %q", previous, cmd.Path, name))
		}
		seen[name] = cmd.Path
	}
	return canon.NewValidationError(problems)
}
