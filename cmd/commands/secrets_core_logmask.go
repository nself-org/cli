package commands

// Purpose: tell the command log which values of this invocation are secret
// (D-0473), so ~/.nself/logs/nself.log never stores a secret argument.
// Inputs: the resolved cobra command and its positional args.
// Outputs: the secret values: positionals the canon marks secret_args, plus
// the values of changed flags the canon marks secret or whose name looks like
// a secret (config.IsSecretKey).
// Constraints: a parent command that swallowed words masks all of them. The
// list comes from internal/canon (memoised), never hand-coded.
// Cheap and best-effort: any lookup problem returns nil, it never fails or
// slows a command. Boolean flags are skipped (their value is not a secret).

import (
	"os"
	"slices"
	"strings"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdlog"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/invoke"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// machineSecretsRestored records whether this invocation restored a bundle.
// Mounted plugins cannot safely accept these values until their protocol
// supports the secret environment hand-off (D-0651).
var machineSecretsRestored bool

// restoreSecretArgv is the first step of Execute: a machine invoker hands the
// secret values over the environment (D-0468); put them back into os.Args and
// tell the command log to mask them (D-0473).
func restoreSecretArgv() error {
	machineSecretsRestored = false
	restored, values, err := invoke.RestoreSecretArgv(os.Args)
	if err != nil {
		return err
	}
	os.Args = restored
	machineSecretsRestored = len(values) > 0
	cmdlog.MaskValues(values...)
	return nil
}

// commandSecretValues returns the secret values of one invocation (nil when
// none or when the command has no canon entry).
func commandSecretValues(cmd *cobra.Command, args []string) (out []string) {
	defer func() {
		if recover() != nil {
			out = nil
		}
	}()
	if cmd == nil {
		return nil
	}
	// The registry is parsed from embedded YAML, which costs milliseconds, so
	// it is read only when a value could need its verdict: a positional word,
	// or a changed flag whose name does not already look secret. A bare
	// `nself version` or `nself status` never reaches it.
	var loaded bool
	var entry canon.Entry
	canonEntry := func() canon.Entry {
		if !loaded {
			loaded = true
			if file, err := canon.Load(); err == nil && file != nil {
				path := strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()))
				entry = file.Commands[path]
			}
		}
		return entry
	}
	add := func(v string) {
		if v != "" {
			out = append(out, v)
		}
	}
	if cmd.HasSubCommands() && len(args) > 0 {
		// A parent swallowed the words (v1.4 `nself config secrets set K V`
		// prints help): the spelling did not resolve, so no canon entry can say
		// which one is the secret. Mask every positional.
		for _, a := range args {
			add(a)
		}
	}
	if len(args) > 0 && len(canonEntry().SecretArgs) > 0 {
		for i, name := range useArgNames(cmd.Use) {
			if i >= len(args) {
				break
			}
			if slices.Contains(entry.SecretArgs, strings.TrimSuffix(name, "...")) {
				add(args[i])
				if strings.HasSuffix(name, "...") {
					for _, rest := range args[i+1:] {
						add(rest)
					}
				}
			}
		}
	}
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if f.Value.Type() == "bool" {
			return
		}
		if !config.IsSecretKey(f.Name) && !canonEntry().Flags[f.Name].Secret {
			return
		}
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			for _, item := range sv.GetSlice() {
				add(item)
			}
			return
		}
		add(f.Value.String())
	})
	return out
}

// useArgNames lists the positional names of a cobra Use string in order, with
// a trailing "..." kept on a variadic one ("<a> [b...]" gives a, b...).
func useArgNames(use string) []string {
	fields := strings.Fields(use)
	if len(fields) < 2 {
		return nil
	}
	var names []string
	for _, tok := range fields[1:] {
		name := strings.Trim(strings.ReplaceAll(tok, "...", ""), "<>[]")
		if strings.Contains(tok, "...") && name != "" {
			name += "..."
		}
		if name == "" || name == "flags" || strings.HasPrefix(name, "-") {
			continue
		}
		names = append(names, name)
	}
	return names
}
