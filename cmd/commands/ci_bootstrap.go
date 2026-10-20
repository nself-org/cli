package commands

// ci_bootstrap.go — Resolving the nself-ci gate binary.
//
// EXCEPTION: ADR 0021 (ungated in 1.4.x): security fix, S-002 + Constitution §7.6; see quality/debt.yaml
//
// Purpose: `nself ci` runs its gates from a separate binary built from the free
//   `ci` plugin. Split out of ci.go (which crossed the 300-line cap) because
//   locating/fetching the gate is a distinct concern from running it.
// Inputs:  the CLI's own executable path, HOME, the configured plugin registry.
// Outputs: an absolute path to a runnable nself-ci binary.
// Constraints: nothing is resolved through PATH and no Go toolchain is used to
//   fetch the gate (the old `go install ...@latest` was unpinned and unverified).
//   The only fetch is autoInstallPlugin("ci", ""), the verified seam: checksum,
//   then release and platform signatures, from the configured registry.
//   The cache-hit path (the published plugin binary) is guarded by ci_bootstrap_test.go.
// SPORT: CLI-CMD-CI-001

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nself-org/cli/internal/plugin"
)

var (
	// ciLegacyOnce makes the legacy-cache notice print once per process.
	ciLegacyOnce sync.Once
	// ciNoticeOut receives that notice; tests swap it.
	ciNoticeOut io.Writer = os.Stderr
	// ciExecutable locates the running CLI for the adjacent dev-build lookup; tests swap it.
	ciExecutable = os.Executable
)

// ensureCIBinary returns the path to the nself-ci binary.
//
// Order: the published plugin binary (~/.nself/plugins/bin), then a dev build
// from an adjacent plugins checkout, then the verified auto-install seam.
// PATH is never consulted: a stale or planted nself-ci there must not win.
func ensureCIBinary(verbose bool) (string, error) {
	retireLegacyCICache()

	// Without a home directory the plugin bin dir falls back to a shared /tmp
	// path that any local user can pre-create: never trust or install there.
	if _, err := os.UserHomeDir(); err != nil {
		return "", fmt.Errorf("cannot locate the home directory for the nself-ci gate: %w\n(install it by hand: nself add ci)", err)
	}

	if p := plugin.PublishedBinaryPath("nself-ci"); isRegularFile(p) {
		return p, nil
	}

	// Locate the plugin source relative to this binary.
	// The CLI binary lives at, e.g., ~/Sites/nself/cli/nself (dev) or
	// /usr/local/bin/nself (installed). In dev the plugin source is adjacent.
	exe, err := ciExecutable()
	if err != nil {
		return "", fmt.Errorf("cannot determine executable path: %w", err)
	}
	exe, _ = filepath.EvalSymlinks(exe)

	candidates := []string{
		// Dev: cli/ → parent nself/ → plugins/free/ci
		filepath.Join(filepath.Dir(exe), "..", "plugins", "free", "ci"),
		filepath.Join(filepath.Dir(exe), "..", "..", "plugins", "free", "ci"),
	}

	var pluginDir string
	for _, c := range candidates {
		abs, _ := filepath.Abs(c)
		if fileExists(filepath.Join(abs, "cmd", "main.go")) {
			pluginDir = abs
			break
		}
	}

	// Installed CLI: the plugin source is not adjacent, because only the single
	// `nself` binary was distributed. Install the signed ci plugin through the
	// verified seam instead of telling the user to clone a second repo.
	if pluginDir == "" {
		if verbose {
			fmt.Fprintln(os.Stderr, "[nself-ci] gate not installed; installing the signed ci plugin")
		}
		return autoInstallPlugin("ci", "")
	}

	binary := filepath.Join(pluginDir, "nself-ci")

	// Build if binary is missing or source is newer.
	if needsBuild(binary, pluginDir) {
		if verbose {
			fmt.Fprintf(os.Stderr, "[nself-ci] building gate binary from %s\n", pluginDir)
		}
		var stderr bytes.Buffer
		build := exec.Command("go", "build", "-o", binary, "./cmd/")
		build.Dir = pluginDir
		build.Stderr = &stderr
		if err := build.Run(); err != nil {
			return "", fmt.Errorf("go build failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
		}
	}
	return binary, nil
}

// ciLegacyCacheBinary returns where earlier CLIs cached an unverified gate
// build (`go install` of the plugin module, or the manual `go build -o` the old
// error text recommended). It is never resolved any more.
func ciLegacyCacheBinary() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".nself", "bin", "nself-ci"), nil
}

// retireLegacyCICache moves a leftover regular-file legacy gate aside to
// nself-ci.unverified-legacy and prints one notice per process. nself never
// deletes state it did not create (Constitution §1.3): a symlink, a directory
// or any other type is left untouched, and a failed rename is not an error.
func retireLegacyCICache() {
	ciLegacyOnce.Do(func() {
		legacy, err := ciLegacyCacheBinary()
		if err != nil {
			return
		}
		info, err := os.Lstat(legacy)
		if err != nil {
			return
		}
		if info.Mode().IsRegular() {
			// Link then unlink, not rename: a link never replaces an existing aside
			// copy, even one created after the Lstat above.
			aside := legacy + ".unverified-legacy"
			if os.Link(legacy, aside) == nil {
				_ = os.Remove(legacy)
			}
		}
		_, _ = fmt.Fprintln(ciNoticeOut, "notice: ~/.nself/bin/nself-ci is no longer used (unverified); nself now installs the signed ci plugin")
	})
}

// isRegularFile reports whether path is itself a regular file. A symlink does
// not count: the installer publishes a copy, so a link there was not put by a
// verified install and falls through to the seam, which replaces it.
func isRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// needsBuild returns true if the binary is missing or any source file is newer.
func needsBuild(binary, pluginDir string) bool {
	info, err := os.Stat(binary)
	if err != nil {
		return true // binary missing
	}
	sources := []string{
		filepath.Join(pluginDir, "cmd", "main.go"),
		filepath.Join(pluginDir, "internal", "gate.go"),
		filepath.Join(pluginDir, "internal", "status.go"),
	}
	for _, src := range sources {
		si, err := os.Stat(src)
		if err != nil {
			continue
		}
		if si.ModTime().After(info.ModTime()) {
			return true
		}
	}
	return false
}

// fileExists returns true if the path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
