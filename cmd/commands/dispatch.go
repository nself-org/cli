package commands

// Command dispatch: how an invocation is resolved before and after cobra.
//
// Purpose: keep root.go to defining the root command and its lifecycle hooks,
// and put the three things that decide WHICH command runs here — the spelling
// the user actually typed, the notice for a command that moved to a plugin,
// and Execute's routing of unknown commands to the plugin proxy.
//
// Inputs: os.Args and the cobra tree.
//
// Outputs: the executed command's error, or a routed plugin invocation.
//
// Constraints: the legacy-spelling rewrite must run before the plugin proxy —
// a retired name is no longer registered, so the proxy would otherwise try to
// resolve it as a plugin.

import (
	"errors"
	"io"
	"strings"

	"fmt"
	"github.com/spf13/pflag"
	"os"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin"

	"github.com/spf13/cobra"
)

// invokedCommandPath returns the command path as the user actually spelled it,
// which is what the deprecation registry is keyed on.
//
// cmd.CommandPath() alone is wrong for an aliased invocation: `nself up` resolves
// to startCmd, whose CommandPath is "nself start". Keying on that would either
// never warn about the old spelling (registry entry "nself up" unreachable) or
// warn on every correct `nself start` (entry "nself start" always matching).
// cobra records the spelling that selected the command in CalledAs(), so an
// alias gets its own registry key while the canonical name stays silent.
func invokedCommandPath(cmd *cobra.Command) string {
	called := cmd.CalledAs()
	if called == "" || called == cmd.Name() {
		return cmd.CommandPath()
	}
	prefix := "nself"
	if parent := cmd.Parent(); parent != nil {
		prefix = parent.CommandPath()
	}
	return prefix + " " + called
}

// warnRelocatedCommand emits the deprecation notice for a command that has
// moved out of core into a plugin.
//
// The registry is keyed on the spelling the user typed, same as
// invokedCommandPath, but this path runs before cobra: an extracted command is
// no longer registered, so RootCmd.Execute never reaches PersistentPreRunE for
// it. Without this the entry exists only as documentation and the user is told
// nothing about where the command went.
// relocatedCommand looks up a command that moved out of core, returning the
// install hint the registry names and whether an entry exists.
//
// The hint has to come from the registry rather than be assembled from the
// command name, because a plugin is not always named after its command:
// `claw` lives in a plugin called `claw-cli`, since a paid `claw` service
// plugin already owns that name. Deriving it produced two contradictory
// instructions one line apart.
//
// Kept separate from printing the warning so that suppressing the warning does
// not also lose the accurate hint — `--no-deprecation-warnings` silences
// deprecation notices, but the proxy's "no such plugin" error is not one.
func relocatedCommand(cmdName string) (installHint string, ok bool) {
	if deprecationRegistry == nil {
		return "", false
	}
	item, found := deprecationRegistry.Lookup("nself " + cmdName)
	if !found {
		return "", false
	}
	return item.Replacement, true
}

// warnRelocatedCommand prints the "this moved to a plugin" notice, and reports
// whether it printed. The caller needs to know: when the notice is silenced,
// the install hint it carries has to reappear in the proxy's error instead.
func warnRelocatedCommand(cmdName string) bool {
	if deprecationRegistry == nil {
		return false
	}
	for _, a := range os.Args {
		if a == "--no-deprecation-warnings" || a == "--quiet" {
			return false
		}
	}
	item, ok := deprecationRegistry.Lookup("nself " + cmdName)
	if !ok {
		return false
	}
	deprecationRegistry.Warn(os.Stderr, item)
	return true
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main().
func Execute() error {
	// Group the command tree for help output. Done here, not in init(): every
	// command's own init() must have registered it on RootCmd first.
	ApplyCommandGroups()

	// Product-alias shim: 'nsentry <args>' (symlinked binary) ≡ 'nself sentry <args>'.
	normalizeInvokedBinary()

	// CLI-R09: rewrite retired top-level spellings onto their new home before
	// anything else looks at os.Args. This has to precede the plugin proxy
	// below: a retired name is no longer a registered command, so the proxy
	// would otherwise try to resolve it as a plugin.
	if legacy := rewriteLegacyInvocation(); legacy != "" {
		warnLegacySpelling(legacy)
	}

	// Route cobra error/usage output to stderr so structured output stays clean.
	RootCmd.SetErr(os.Stderr)

	// Wrap every command with the --json guard and coded usage errors.
	installInvocationDecorator(RootCmd)

	// Intercept unknown commands for the plugin router
	if len(os.Args) > 1 {
		cmdName := os.Args[1]

		// Ignore global flags or root help
		if cmdName != "" && cmdName != "help" && cmdName[0] != '-' {
			// Check if the command is known to Cobra
			isKnown := false
			for _, c := range RootCmd.Commands() {
				if c.Name() == cmdName || c.HasAlias(cmdName) {
					isKnown = true
					break
				}
			}

			if !isKnown {
				// A command that left core for a plugin (CLI-R11) still has a
				// deprecation registry entry naming where it went. cobra never
				// sees the invocation — it is not a registered command — so
				// PersistentPreRunE's warning never runs and the entry would be
				// decorative. Emit it here, on the one path that does see it.
				// ...but ONLY when the plugin is absent: once the user has run
				// `nself install soak`, `nself soak` IS the supported spelling,
				// and telling them to install what they just installed is noise.
				// Three facts decide what the user is told, and they are
				// independent: whether the command moved, whether the plugin is
				// already installed, and whether deprecation notices are
				// silenced.
				registryHint, relocated := relocatedCommand(cmdName)
				installed := plugin.IsCommandInstalled(cmdName)

				installHint := "nself install " + cmdName
				switch {
				case installed:
					// The old spelling IS the supported spelling now. Nothing to
					// warn about, and nothing to suggest installing.
					installHint = ""
				case relocated:
					// The registry knows the real plugin name, which is not
					// always the command name (`claw` lives in `claw-cli`).
					if warnRelocatedCommand(cmdName) {
						// The warning just said it; the proxy stays quiet
						// rather than repeating it in different words.
						installHint = ""
					} else {
						// Warnings are silenced, so the proxy's error is the
						// only place the user can learn the right name.
						installHint = registryHint
					}
				}

				// Proxy to plugin.
				//
				// The CLI's own persistent flags are dropped first. Before this
				// command moved to a plugin, cobra consumed them at the root and
				// the subcommand never saw them; passing them through now makes
				// `nself <cmd> --no-deprecation-warnings ...` die with
				// "unknown flag" in a plugin that has no reason to know about
				// them. Stripping reproduces the pre-extraction behaviour.
				pluginArgs := []string{}
				if len(os.Args) > 2 {
					pluginArgs = stripRootPersistentFlags(os.Args[2:])
				}
				if err := plugin.ProxyCommandWithHint(cmdName, pluginArgs, installHint); err != nil {
					return reportProxyFailure(os.Stderr, err)
				}
				return nil
			}
		}
	}

	if err := RootCmd.Execute(); err != nil {
		return err
	}
	// Some commands (status, health) succeed but request a non-zero status to
	// report the state they found. Surface it as a silent exit error so main()
	// stays the only os.Exit caller.
	if ctx := RootCmd.Context(); ctx != nil {
		if code, ok := ctx.Value(exitCodeKey).(int); ok && code != 0 {
			return errs.Exit(code)
		}
	}
	return nil
}

// stripRootPersistentFlags removes the CLI's own persistent flags from an
// argument list bound for a plugin binary.
//
// Derived from RootCmd rather than hardcoded, so a flag added to the CLI later
// does not silently start breaking every extracted command.
//
// A plugin that happens to define a flag of the same name loses it. That is the
// correct trade: the flag belonged to nself before the command moved, and a
// script that passed it was always talking to nself, not to the subcommand.
func stripRootPersistentFlags(args []string) []string {
	type flagInfo struct{ takesValue bool }
	known := map[string]flagInfo{}
	RootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		// --json is forwarded: the plugin owns its output (invocation.go).
		if keepsForPlugin(f.Name) {
			return
		}
		// A bool flag never consumes the following argument; anything else does
		// when written as "--flag value" rather than "--flag=value".
		known["--"+f.Name] = flagInfo{takesValue: f.Value.Type() != "bool"}
		if f.Shorthand != "" {
			known["-"+f.Shorthand] = flagInfo{takesValue: f.Value.Type() != "bool"}
		}
	})

	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]

		// Everything after "--" is positional by convention; pass it verbatim.
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}

		name := a
		hasInlineValue := false
		if eq := strings.IndexByte(a, '='); eq > 0 {
			name = a[:eq]
			hasInlineValue = true
		}

		info, isRootFlag := known[name]
		if !isRootFlag {
			out = append(out, a)
			continue
		}
		if info.takesValue && !hasInlineValue {
			i++ // drop the separate value token too
		}
	}
	return out
}

// reportProxyFailure turns a plugin proxy error into an exit status, printing
// only when the user has not already been told.
//
// A plugin that ran and failed wrote its own error to the inherited stderr.
// Printing "Plugin error: plugin exited with code 1" underneath it repeats,
// less usefully, what is already on screen:
//
//	Error: use --force to bypass the PLANNED gate
//	Plugin error: plugin exited with code 1
//
// ExitCodeError has advertised Silent() since it was written; this is the call
// site that ignored it. A silent error mirrors the plugin's status and says
// nothing. Anything else is the proxy's own failure — the plugin is missing, or
// could not be executed — where the user has seen no output at all and needs
// the message.
func reportProxyFailure(w io.Writer, err error) error {
	var silent errs.Silencer
	if errors.As(err, &silent) && silent.Silent() {
		code := 1
		var coder errs.ExitCoder
		if errors.As(err, &coder) {
			code = coder.ExitCode()
		}
		return errs.Exit(code)
	}

	_, _ = fmt.Fprintf(w, "Plugin error: %v\n", err)
	return errs.Exit(1)
}
