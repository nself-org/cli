package manifestv2

import (
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// CoreVerbs are the ADR 0016 core command verbs a plugin may not claim (E113).
// TestCoreVerbsMatchCanon keeps this list equal to internal/canon.Verbs.
var CoreVerbs = []string{"init", "start", "stop", "restart", "status", "logs", "doctor", "build", "reset",
	"clean", "add", "remove", "config", "db", "backup", "deploy", "update", "license", "mcp", "exec"}

var (
	segmentRe = regexp.MustCompile(NamePattern)
	binaryRe  = regexp.MustCompile(BinaryPattern)
)

func validateCommands(m *Manifest) error {
	c := m.Commands
	if c == nil {
		return nil
	}
	if !segmentRe.MatchString(c.Command) {
		return invalid("commands.command", "must match "+NamePattern)
	}
	if in(CoreVerbs, c.Command) {
		return errs.Newf("E113", "commands.command %q is a core command verb and cannot be mounted by plugin %q", c.Command, m.Name)
	}
	if !binaryRe.MatchString(c.Binary) {
		return invalid("commands.binary", "must match "+BinaryPattern)
	}
	if err := checkAttrs("commands", c.SideEffect, c.Output, c.JSON, c.Surface); err != nil {
		return err
	}
	if err := checkConfirm("commands", c.Confirm, nil); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, s := range c.Subcommands {
		p := "commands.subcommands[" + itoa(i) + "]"
		if err := validateSub(p, s, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateSub(p string, s Subcommand, seen map[string]bool) error {
	if strings.TrimSpace(s.Name) == "" {
		return invalid(p+".name", "is required")
	}
	for _, seg := range strings.Split(s.Name, " ") {
		if !segmentRe.MatchString(seg) {
			return invalidf(p+".name", "segment %q must match %s", seg, NamePattern)
		}
	}
	if seen[s.Name] {
		return invalidf(p+".name", "%q is declared twice", s.Name)
	}
	seen[s.Name] = true
	if err := checkAttrs(p, s.SideEffect, s.Output, s.JSON, s.Surface); err != nil {
		return err
	}
	for i, a := range s.Args {
		if !segmentRe.MatchString(a.Name) {
			return invalid(p+".args["+itoa(i)+"].name", "must match "+NamePattern)
		}
	}
	for i, f := range s.Flags {
		fp := p + ".flags[" + itoa(i) + "]"
		if !segmentRe.MatchString(f.Name) {
			return invalid(fp+".name", "must match "+NamePattern)
		}
		if f.Type == "" {
			return invalid(fp+".type", "is required")
		}
		if err := checkAttrs(fp, f.SideEffect, f.Output, f.JSON, ""); err != nil {
			return err
		}
	}
	return checkConfirm(p, s.Confirm, s.Flags)
}

// checkAttrs validates the optional enum attributes; empty means absent.
func checkAttrs(p, side, out, js, surface string) error {
	for _, e := range []struct {
		key, val string
		set      []string
	}{{"side_effect", side, SideEffects}, {"output", out, Outputs}, {"json", js, JSONModes}, {"surface", surface, Surfaces}} {
		if e.val != "" && !in(e.set, e.val) {
			return invalidf(p+"."+e.key, "must be one of %s, got %q", strings.Join(e.set, ", "), e.val)
		}
	}
	return nil
}

// checkConfirm validates confirm: every flag and plan.flag names a declared
// bool flag of the same command, plan.id_flag a declared string flag. The root
// declares no flags, so only an empty list is valid there.
func checkConfirm(p string, c *Confirm, flags []Flag) error {
	if c == nil {
		return nil
	}
	if c.Flags == nil {
		return invalid(p+".confirm.flags", "is required (an empty list is valid)")
	}
	find := func(name string) *Flag {
		for i := range flags {
			if flags[i].Name == name {
				return &flags[i]
			}
		}
		return nil
	}
	for j, name := range c.Flags {
		f := find(name)
		if f == nil || f.Type != "bool" {
			return invalidf(p+".confirm.flags["+itoa(j)+"]", "%q must be a declared bool flag of this command", name)
		}
	}
	if c.Plan != nil {
		if f := find(c.Plan.Flag); f == nil || f.Type != "bool" {
			return invalidf(p+".confirm.plan.flag", "%q must be a declared bool flag of this command", c.Plan.Flag)
		}
		if f := find(c.Plan.IDFlag); f == nil || f.Type != "string" {
			return invalidf(p+".confirm.plan.id_flag", "%q must be a declared string flag of this command", c.Plan.IDFlag)
		}
	}
	return nil
}
