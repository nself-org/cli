package commands

// Break-out relocation (EPIC P7-CANON D10, P7-CANON-13) and the relocated-command
// lookup the unknown-command intercept shares.
//
// Purpose: in v1.5 mode a command that left core for a plugin (ci, runner,
// functions, server, plugin init|new|dev|debug|link|unlink|test) must still run
// the first time it is typed. When its plugin is not mounted, the verified
// auto-install seam (autoInstallPlugin, P7-CANON-23) installs it at the
// lock-pinned version (else the catalogue's), one warning is printed, and the
// binary runs. `nself ci` is every repo's merge gate, so relocation never
// strands a gate: offline, refused, unsigned or tampered all fail loudly.
//
// Inputs: the command word and the arguments after it, the generated break-out
// table, the prepared cobra tree (to learn whether the plugin is mounted),
// nself.lock.yaml, NSELF_NO_AUTO_ADD.
//
// Outputs: handled=false when the invocation is not a break-out or the plugin
// is mounted (cobra then runs it); otherwise handled=true and the plugin's exit
// status, or E404 with Fix `nself add <slug>`.
//
// Constraints: v1.5 only (the caller checks); installs only through the seam
// (strict signature, checksum, configured registry, free tier); a plugin the
// user disabled is never reinstalled; one stderr warning per process, silenced
// by --quiet and --no-deprecation-warnings; the exec goes through
// plugin.ProxyBinary, which builds the child environment with plugin.ExecEnv
// (contract:cli.plugin-exec-env v1).

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/plugin"
)

// envNoAutoAdd turns the break-out auto-add off: the command fails with E404.
const envNoAutoAdd = "NSELF_NO_AUTO_ADD"

// autoAddPlugin is the install seam and relocateWarnOut the warning's writer;
// tests swap both.
var (
	autoAddPlugin             = autoInstallPlugin
	relocateWarnOut io.Writer = os.Stderr
)

// relocateBreakout handles `nself <cmdName> <args...>` when it names a break-out
// whose plugin is not mounted.
func relocateBreakout(cmdName string, args []string) (handled bool, err error) {
	row, ok := breakoutRow(cmdName, args)
	if !ok || mountedBy(walkNames(RootCmd, row.To[:1]), row.Plugin) {
		return false, nil
	}
	old := strings.Join(row.From, " ")
	if os.Getenv(envNoAutoAdd) == "1" {
		return true, breakoutRefused(old, row.Plugin, errors.New("automatic install is switched off ("+envNoAutoAdd+"=1)"))
	}
	if _, err := os.Stat(filepath.Join(resolvePluginDir(), row.Plugin, ".disabled")); err == nil {
		return true, errs.Newf("E407", "plugin %q is disabled, so 'nself %s' cannot run", row.Plugin, old).
			WithFix("Run nself add " + row.Plugin + " to restore it.")
	}
	version, err := lockedPluginVersion(row.Plugin)
	if err != nil {
		return true, err
	}
	bin, err := autoAddPlugin(row.Plugin, version)
	if err != nil {
		return true, breakoutRefused(old, row.Plugin, err)
	}
	if !warningsSilenced(os.Args) {
		installed := readInstalledVersion(resolvePluginDir(), row.Plugin)
		_, _ = fmt.Fprintf(relocateWarnOut, "warning: 'nself %s' now comes from the %s plugin; installed %s@%s\n", old, row.Plugin, row.Plugin, installed)
	}
	// The old spelling's tail goes to the binary behind the row's remaining
	// words ("runner verify x" -> "nodes verify x"), minus the CLI's own flags.
	pluginArgs := append(append([]string{}, row.To[1:]...), stripRootPersistentFlags(args[len(row.From)-1:])...)
	if err := plugin.ProxyBinary(filepath.Base(bin), row.Plugin, pluginArgs); err != nil {
		return true, reportProxyFailure(os.Stderr, err)
	}
	return true, nil
}

// breakoutRow returns the longest break-out row whose From starts with cmdName
// and continues with the leading words of args.
func breakoutRow(cmdName string, args []string) (best canonRowT, ok bool) {
	words := []string{cmdName}
	for _, a := range args {
		if a == "" || a[0] == '-' {
			break
		}
		words = append(words, a)
	}
	for _, r := range canonTable.Breakouts {
		if len(r.To) > 0 && hasPrefix(words, r.From) && (!ok || len(r.From) > len(best.From)) {
			best, ok = r, true
		}
	}
	return best, ok
}

// lockedPluginVersion is the version nself.lock.yaml pins for slug, or "" (the
// catalogue's current) when the project has no lock or the lock does not pin it.
func lockedPluginVersion(slug string) (string, error) {
	root, err := projectRoot()
	if err != nil {
		return "", nil
	}
	l, err := lock.Load(root)
	if errors.Is(err, lock.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", errs.New("E455", err.Error()).WithFix("Run nself build to rewrite " + lock.FileName + ", then run the command again.")
	}
	v, _ := l.PluginVersion(slug)
	return v, nil
}

// breakoutRefused is the E404 of a break-out the CLI could not install: offline,
// refused, unsigned, tampered or switched off.
func breakoutRefused(old, slug string, cause error) error {
	return errs.Newf("E404", "'nself %s' now comes from the %s plugin, which is not installed and was not added: %v", old, slug, cause).
		WithFix("Run `nself add " + slug + "`, then run the command again.")
}

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
	if warningsSilenced(os.Args) {
		return false
	}
	item, ok := deprecationRegistry.Lookup("nself " + cmdName)
	if !ok {
		return false
	}
	deprecationRegistry.Warn(os.Stderr, item)
	return true
}
