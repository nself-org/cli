package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// ExitCodeError is returned when a plugin process exits with a non-zero code.
// main() reads ExitCode() and mirrors the plugin's status; the message is not
// printed, because the plugin already wrote its own output to the terminal.
type ExitCodeError struct {
	Code int
}

func (e *ExitCodeError) Error() string {
	return fmt.Sprintf("plugin exited with code %d", e.Code)
}

// ExitCode satisfies the interface main() and errs.ExitCodeFor look for.
func (e *ExitCodeError) ExitCode() int { return e.Code }

// Silent reports that main() must not print this error: the plugin subprocess
// inherited stdout/stderr and has already reported the failure itself.
func (e *ExitCodeError) Silent() bool { return true }

// pluginBinDir returns the directory where plugin binaries are installed.
// This is ~/.nself/plugins/bin/ (or /tmp/.nself/plugins/bin/ as fallback).
func pluginBinDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", ".nself", "plugins", "bin")
	}
	return filepath.Join(home, ".nself", "plugins", "bin")
}

// ProxyCommand checks if a plugin binary exists and executes it.
// If not, it instructs the user to install it.
//
// For security, the binary is looked up ONLY in the plugin bin directory,
// never via the full system PATH. This prevents PATH hijacking attacks.
func ProxyCommand(cmdName string, args []string) error {
	return ProxyCommandWithHint(cmdName, args, "nself install "+cmdName)
}

// ProxyCommandWithHint is ProxyCommand with control over the install hint in
// the not-found error.
//
// The generic hint assumes the plugin is named after the command, which is not
// always true: `claw` lives in a plugin called `claw-cli`, because a paid
// `claw` service plugin already owns that name. When the deprecation registry
// has already told the user the right thing to install, passing an empty hint
// here stops the proxy contradicting it — the user was being shown
// "use 'nself install claw-cli'" and "nself install claw" one line apart.
func ProxyCommandWithHint(cmdName string, args []string, installHint string) error {
	pluginBinary := fmt.Sprintf("nself-%s", cmdName)

	// S-002: Only look in the plugin bin directory, never the full PATH.
	binDir := pluginBinDir()
	candidate := filepath.Join(binDir, pluginBinary)
	if runtime.GOOS == "windows" {
		candidate += ".exe"
	}

	if _, err := os.Stat(candidate); err != nil {
		// CLI-R19: an unknown command is the moment a user most needs to be told
		// how to get it, so the actionable message — `nself install X` — goes in
		// the returned error.
		//
		// It deliberately does NOT also go to slog. The CLI never calls
		// slog.SetDefault, so slog.Warn here reached the user as a raw
		// timestamped line above the real error:
		//
		//   2026/08/25 10:16:19 WARN plugin binary not found command=gateway ...
		//   Plugin error: unknown command "gateway" ...
		//
		// which is both duplicated and the wrong register for a terminal. An
		// earlier comment claimed a normal terminal never shows it; running the
		// binary showed otherwise.
		if installHint == "" {
			return fmt.Errorf("unknown command %q, and no plugin named %q is installed", cmdName, cmdName)
		}
		return fmt.Errorf("unknown command %q, and no plugin named %q is installed.\n\nIf this is an nSelf plugin, install it with:\n  %s",
			cmdName, cmdName, installHint)
	}

	return execPluginBinary(candidate, cmdName, args)
}

// ProxyBinary execs the named plugin binary with args verbatim. It is the
// exec path of the plugin-command mount (contract:cli.plugin-command-mount
// v1): the binary is resolved ONLY inside the plugin bin directory (S-002),
// argv is passed straight through with no shell, stdio is inherited and a
// non-zero plugin exit status comes back as ExitCodeError. slug names the
// plugin whose manifest (if present) declares project settings for the child
// environment, exactly like the unknown-command proxy.
//
// The mount validates the binary at discovery time (mount.Discover, E406);
// a binary that vanishes between discovery and exec fails with E406 here.
func ProxyBinary(binName, slug string, args []string) error {
	binDir := pluginBinDir()
	if !safeBinaryName(binName) {
		return errs.Newf("E406", "plugin %q binary %q has an invalid name", slug, binName)
	}
	root, err := os.OpenRoot(filepath.Dir(binDir))
	if err != nil {
		return errs.Newf("E406", "plugin %q binary directory is unavailable", slug)
	}
	defer func() { _ = root.Close() }()
	fileName := binName
	if runtime.GOOS == "windows" {
		fileName += ".exe"
	}
	f, err := root.Open(filepath.Join("bin", fileName))
	if err != nil {
		return errs.Newf("E406", "plugin %q binary %q is not present in %s; reinstall it with nself add %s", slug, binName, binDir, slug)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0) {
		return errs.Newf("E406", "plugin %q binary %q is not executable", slug, binName)
	}
	if runtime.GOOS == "windows" {
		return execPluginBinary(filepath.Join(binDir, fileName), slug, args)
	}
	if runtime.GOOS == "darwin" {
		return execPinnedBinary(f, filepath.Join(binDir, fileName), slug, args)
	}
	return execPluginBinaryFile(f, filepath.Join(binDir, fileName), slug, args)
}

// execPinnedBinary creates a private hard link to the opened executable on
// macOS, where /dev/fd cannot be passed to execve. Comparing inode identity
// after linking catches a replacement between the rooted open and the link.
func execPinnedBinary(f *os.File, candidate, slug string, args []string) error {
	dir, err := os.MkdirTemp(filepath.Dir(filepath.Dir(candidate)), ".mount-")
	if err != nil {
		return errs.Newf("E406", "plugin %q binary could not be pinned: %v", slug, err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return errs.Newf("E406", "plugin %q binary changed before execution", slug)
	}
	pinned := filepath.Join(dir, filepath.Base(candidate))
	if err := os.Link(resolved, pinned); err != nil {
		return errs.Newf("E406", "plugin %q binary could not be pinned: %v", slug, err)
	}
	opened, err := f.Stat()
	if err != nil {
		return errs.Newf("E406", "plugin %q binary changed before execution", slug)
	}
	linked, err := os.Stat(pinned)
	if err != nil || !os.SameFile(opened, linked) {
		return errs.Newf("E406", "plugin %q binary changed before execution", slug)
	}
	return execPluginBinary(pinned, slug, args)
}

func safeBinaryName(name string) bool {
	if !strings.HasPrefix(name, "nself-") || len(name) <= len("nself-") {
		return false
	}
	if name[len("nself-")] < 'a' || name[len("nself-")] > 'z' {
		return false
	}
	for _, r := range name[len("nself-"):] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

// execPluginBinary is the shared run of a plugin binary: inherited stdio, the
// project settings the plugin's manifest declares, and the plugin's exit
// status surfaced as ExitCodeError (silent: the child already reported).
func execPluginBinary(path, slug string, args []string) error {
	// Prepare the command.
	//
	// The plugin inherits this process's environment, plus whichever project
	// settings its manifest declares. Without the latter a command plugin sees
	// nothing from the project's .env cascade — it runs on the user's machine,
	// not in a container compose has populated — and would have to re-implement
	// the cascade that CLI-R18 made canonical.
	cmd := exec.Command(path, args...)
	return runPluginCommand(cmd, slug)
}

// execPluginBinaryFile executes the already validated open file descriptor.
// The child inherits fd 3, so replacing the bin symlink after validation
// cannot change which executable runs.
func execPluginBinaryFile(f *os.File, displayPath, slug string, args []string) error {
	cmd := exec.Command("/dev/fd/3", args...)
	cmd.Args[0] = displayPath
	cmd.ExtraFiles = []*os.File{f}
	return runPluginCommand(cmd, slug)
}

func runPluginCommand(cmd *exec.Cmd, slug string) error {
	if extra := pluginEnvForCommand(slug); len(extra) > 0 {
		cmd.Env = append(os.Environ(), extra...)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run process
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return &ExitCodeError{Code: exitError.ExitCode()}
		}
		return fmt.Errorf("failed to execute plugin: %w", err)
	}

	return nil
}

// pluginEnvForCommand resolves the declared project settings for an installed
// plugin providing cmdName.
//
// Reads the plugin's own manifest from where it was installed. A missing or
// unreadable manifest means nothing is declared, so nothing is passed —
// degrading to the previous behaviour rather than failing the invocation.
func pluginEnvForCommand(cmdName string) []string {
	// Installed plugins sit alongside the bin directory the proxy reads, so
	// deriving the path from pluginBinDir keeps the two from drifting apart.
	installed := filepath.Join(filepath.Dir(pluginBinDir()), cmdName)
	m := readPluginManifest(installed)
	if m == nil {
		return nil
	}
	return PluginEnv(".", m)
}
