package commands

// Invocation decorator: guard --json, record the invocation, isolate envelope
// stdout, code usage errors, and lock project mutations. The pre-run guard
// precedes command hooks; the body guard backs it up. Plain human runs do not
// resolve the registry unless --format json was selected.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/observability"
	"github.com/nself-org/cli/internal/oplock/oplockcmd"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// decorated records the commands already wrapped, so installing twice (tests,
// repeated Execute in one process) never stacks wrappers.
var decorated sync.Map

// jsonEntryFor resolves the registry entry of a command. A variable so tests
// can supply a fixture registry for a fixture tree.
var jsonEntryFor = func(cmd *cobra.Command) (*cmdregistry.Command, error) {
	reg, err := commandRegistry()
	if err != nil {
		return nil, err
	}
	e, ok := reg.Lookup(cmd.CommandPath())
	if !ok {
		return nil, fmt.Errorf("command registry has no entry for %q", cmd.CommandPath())
	}
	return e, nil
}

// keepsForPlugin reports whether a root persistent flag is forwarded to a
// plugin binary instead of being stripped. --json is: a plugin-proxied command
// owns its output, so it must see the flag (contract:cli.json-envelope).
func keepsForPlugin(name string) bool { return name == "json" }

// installInvocationDecorator wraps every command under root exactly once.
func installInvocationDecorator(root *cobra.Command) {
	// cobra adds `help` only inside Execute; wrap it too.
	root.InitDefaultHelpCmd()
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return codedUsageError(err) })
	var rec func(c *cobra.Command)
	rec = func(c *cobra.Command) {
		decorate(c)
		for _, ch := range c.Commands() {
			rec(ch)
		}
	}
	rec(root)
}

// decorate wraps one command's Args and RunE/Run.
func decorate(c *cobra.Command) {
	if _, done := decorated.LoadOrStore(c, struct{}{}); done {
		return
	}
	switch {
	case c.PersistentPreRunE != nil:
		c.PersistentPreRunE = guardedPre(c.PersistentPreRunE)
	case c.PersistentPreRun != nil:
		pre := c.PersistentPreRun
		c.PersistentPreRun = nil
		c.PersistentPreRunE = guardedPre(func(cmd *cobra.Command, args []string) error {
			pre(cmd, args)
			return nil
		})
	case c.Parent() == nil:
		c.PersistentPreRunE = guardedPre(func(*cobra.Command, []string) error { return nil })
	}
	if c.Run == nil && c.RunE == nil {
		return // hub without a body: cobra prints help, nothing to guard
	}
	if c.Annotations["nself.mount.source"] == "installed" {
		return
	}
	if c.Args != nil {
		orig := c.Args
		c.Args = func(cmd *cobra.Command, args []string) error {
			return codedUsageError(orig(cmd, args))
		}
	}
	switch {
	case c.RunE != nil:
		c.RunE = guarded(locked(c.RunE))
	default:
		run := c.Run
		c.Run = nil
		c.RunE = guarded(locked(func(cmd *cobra.Command, args []string) error {
			run(cmd, args)
			return nil
		}))
	}
}

// guarded records the invocation, refuses unsupported --json, then calls the body.
func guarded(orig func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := enterInvocation(cmd); err != nil {
			return err
		}
		if shouldIsolateJSON(cmd) {
			restore := output.IsolateStdout()
			defer restore()
		}
		return runWithUpdateHint(cmd, args, orig)
	}
}

func shouldIsolateJSON(cmd *cobra.Command) bool {
	if cmd.Parent() == nil || cmd.Annotations["nself.mount.source"] == "installed" {
		return false
	}
	on, err := jsonModeOf(cmd)
	if err != nil || !on {
		return false
	}
	e, err := jsonEntryFor(cmd)
	if err != nil {
		return false
	}
	kind, _ := effectiveJSON(cmd, e)
	return kind == canon.JSONEnvelope
}

// locked wraps a command body with the project operation lock, taken after the
// --json guard and released by a defer on every return and panic. The registry
// is read only inside a project (internal/oplock/oplockcmd).
func locked(orig func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return oplockcmd.Wrap(func(c *cobra.Command) (*cmdregistry.Command, error) { return jsonEntryFor(c) }, orig)
}

// guardedPre checks before hooks can fetch, cache, or change directories.
func guardedPre(orig func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := enterInvocation(cmd); err != nil {
			return err
		}
		return orig(cmd, args)
	}
}

// enterInvocation records the invocation and refuses unsupported --json. It is
// idempotent: the pre-run hook and RunE both call it.
func enterInvocation(cmd *cobra.Command) error {
	on, err := jsonModeOf(cmd)
	if err != nil {
		return err
	}
	output.SetInvocation(invokedKey(cmd), on)
	if on {
		return refuseUnsupportedJSON(cmd)
	}
	return nil
}

// invokedKey is the canonical registry path without the root name ("" for the
// root command).
func invokedKey(cmd *cobra.Command) string {
	return strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), cmd.Root().CommandPath()))
}

// jsonModeOf reports whether JSON mode is on: the bool `json` flag is true, or
// `--format json` was given to a command whose registry json is envelope.
// `--json=false` is off. A non-bool `json` flag is an error, never a default.
func jsonModeOf(cmd *cobra.Command) (bool, error) {
	if f := cmd.Flags().Lookup("json"); f != nil {
		v, err := cmd.Flags().GetBool("json")
		if err != nil {
			return false, fmt.Errorf("--json is not a bool flag on %q: %w", cmd.CommandPath(), err)
		}
		if v {
			return true, nil
		}
	}
	if f := cmd.Flags().Lookup("format"); f != nil && f.Value.String() == "json" && cmd.Parent() != nil {
		e, err := jsonEntryFor(cmd)
		if err != nil {
			return false, fmt.Errorf("--format json: %w", err)
		}
		return e.JSON == canon.JSONEnvelope, nil
	}
	return false, nil
}

// refuseUnsupportedJSON applies EPIC D7 and D12(b) once JSON mode is on.
func refuseUnsupportedJSON(cmd *cobra.Command) error {
	kind, flagName := canon.JSONNone, ""
	if cmd.Parent() != nil { // the root command has no entry: json none
		e, err := jsonEntryFor(cmd)
		if err != nil {
			return fmt.Errorf("--json: %w", err)
		}
		kind, flagName = effectiveJSON(cmd, e)
	}
	if kind != canon.JSONNone {
		return nil
	}
	if !isRootJSONFlag(cmd) {
		// The command or a non-root ancestor declared its own `json` flag long
		// before P7-REG and its body ignores it.
		// compat.V15(P7-REG-05): --json accepted and ignored on a command whose own json flag predates P7-REG -> refused with E402
		if !compat.V15() {
			return nil
		}
	}
	what := "nself " + strings.TrimSpace(invokedKey(cmd))
	what = strings.TrimSpace(what)
	if flagName != "" {
		what += " --" + flagName
	}
	return errs.New("E402", what+" does not support --json")
}

// effectiveJSON returns the command's JSON support after active flag
// overrides, and the flag that caused a `none` override. An active override
// replaces the command value; several active overrides resolve none > legacy.
func effectiveJSON(cmd *cobra.Command, e *cmdregistry.Command) (kind, flag string) {
	kind = e.JSON
	rank := map[string]int{canon.JSONEnvelope: 0, canon.JSONLegacy: 1, canon.JSONNone: 2}
	best := -1
	for _, f := range e.Flags {
		if f.JSON == nil {
			continue
		}
		pf := cmd.Flags().Lookup(f.Name)
		if pf == nil || !pf.Changed || (pf.Value.Type() == "bool" && pf.Value.String() != "true") {
			continue
		}
		if r := rank[*f.JSON]; r > best {
			best, kind, flag = r, *f.JSON, f.Name
		}
	}
	if kind != canon.JSONNone {
		flag = ""
	}
	return kind, flag
}

// isRootJSONFlag reports whether the command's `json` flag is the root
// persistent flag itself (pointer equality): the command had no --json before
// P7-REG, so --json was an exit-1 unknown flag.
func isRootJSONFlag(cmd *cobra.Command) bool {
	f := cmd.Flags().Lookup("json")
	return f != nil && f == cmd.Root().PersistentFlags().Lookup("json")
}

// codedUsageError wraps a flag or argument error as E401 in v1.5 mode and
// returns cobra's error untouched in v1.4 mode (EPIC D12(a)): main prints
// `Error: %v`, and a CLIError's Error() would add [E401] and Why/Fix lines to
// every usage error in 1.4.x. Errors that are already coded or silent pass
// through, as does nil.
func codedUsageError(err error) error {
	if err == nil {
		return nil
	}
	// compat.V15(P7-REG-05): flag and argument errors keep cobra's text -> wrapped as E401
	if !compat.V15() {
		return err
	}
	var coded *errs.CLIError
	var silent errs.Silencer
	if errors.As(err, &coded) || (errors.As(err, &silent) && silent.Silent()) {
		return err
	}
	// The cobra error is not wrapped as the cause: its text can echo argv.
	return errs.New("E401", sanitizeUsageMessage(err.Error()))
}

var (
	// shorthandEcho: `unknown shorthand flag: 'x' in -xsecret` echoes the rest of the argument.
	shorthandEcho = regexp.MustCompile(`( in )-\S*`)
	// badValueEcho: `invalid argument "v" for "--flag" flag: ...` echoes the value.
	badValueEcho = regexp.MustCompile(`invalid argument "([^"]*)" for "(-[^"]*)"`)
	secretFlag   = regexp.MustCompile(`(?i)pass|token|secret|key|auth|cred|licen`)
)

// sanitizeUsageMessage removes user-typed values from a cobra usage error
// before it becomes an E401 message (which reaches stderr and, from v1.5, the
// error envelope): the argument tail of an unknown shorthand, and the value of
// an invalid flag argument. Everything else goes through observability.Redact.
func sanitizeUsageMessage(msg string) string {
	msg = shorthandEcho.ReplaceAllString(msg, "${1}-<redacted>")
	// The value is echoed again by the parser tail (`strconv.ParseInt: parsing
	// "v"`), so a secret-named flag's value is removed everywhere it appears.
	if m := badValueEcho.FindStringSubmatch(msg); m != nil && m[1] != "" && secretFlag.MatchString(m[2]) {
		msg = strings.ReplaceAll(msg, m[1], "<redacted>")
	}
	return observability.Redact(msg)
}
