package commands

// Purpose: Detects whether `nself` is being run from inside its own (or
// any Go CLI's) source repository — identified by a cmd/ + internal/
// layout — and blocks commands that would generate runtime artifacts
// (docker-compose.yml, nginx/, ssl/, .env) there, since that is almost
// always an operator mistake rather than an intended project directory.
// Split out of root.go (CLI-R12) to keep this self-contained safety check
// out of the RootCmd/Execute file.
// Inputs: the current working directory (checkNotInSourceRepo) and a
// command name (isSourceSafeCommand).
// Outputs: an error naming the offending directory, or nil; a bool for
// whether a given command is safe to run from a source tree regardless
// (e.g. --help, version).
// Constraints: pure move — no behavior changes. Execute (root.go) calls
// checkNotInSourceRepo before dispatching to any command not covered by
// isSourceSafeCommand.

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
)

// checkNotInSourceRepo detects if the user is running nself from within the
// nself CLI source repository (or any Go CLI source tree with cmd/ + internal/).
// This prevents generating docker-compose.yml, nginx/, ssl/, .env, and other
// runtime artifacts inside the source directory — a common mistake.
func checkNotInSourceRepo() error {
	cwd, err := os.Getwd()
	if err != nil {
		return nil // can't determine — allow
	}

	// Allow override for testing
	if os.Getenv("NSELF_ALLOW_SOURCE_DIR") == "1" {
		return nil
	}

	// Detect Go CLI source repo: cmd/commands/ + internal/ + go.mod
	markers := []string{
		filepath.Join(cwd, "cmd", "commands"),
		filepath.Join(cwd, "internal"),
		filepath.Join(cwd, "go.mod"),
	}
	allExist := true
	for _, m := range markers {
		if _, err := os.Stat(m); err != nil {
			allExist = false
			break
		}
	}
	if !allExist {
		return nil
	}

	// Additional check: go.mod must contain "module nself"
	data, err := os.ReadFile(filepath.Join(cwd, "go.mod"))
	if err != nil {
		return nil
	}
	if len(data) < 20 {
		return nil
	}
	// Check first line for "module nself"
	firstLine := string(data[:min(len(data), 100)])
	if firstLine == "" {
		return nil
	}
	for _, sig := range []string{"module nself", "github.com/nself-org/cli/cmd", "github.com/nself-org/cli/internal"} {
		if strings.Contains(firstLine, sig) {
			return fmt.Errorf(`cannot run nself commands inside the CLI source repository

You are in: %s

This directory contains the nself CLI source code (cmd/, internal/, go.mod).
Running nself here would generate docker-compose.yml, nginx configs, SSL certs,
and other runtime artifacts inside the source tree.

To fix:
  1. cd to your actual project directory
  2. Run: nself init
  3. Then: nself start

To override (testing only): export NSELF_ALLOW_SOURCE_DIR=1`, cwd)
		}
	}

	return nil
}

// isSourceSafeCommand returns true for commands that are safe to run from
// anywhere, including the source repository.
func isSourceSafeCommand(name string) bool {
	if isRepoScopedCommand(name) {
		return true
	}
	safe := map[string]bool{
		"help": true, "version": true, "completion": true,
		"update": true, "upgrade": true, "doctor": true, "nself": true,
	}
	return safe[name]
}

// isRepoScopedCommand returns true for commands that operate on a source
// repository rather than on a running nSelf stack.
//
// These must be exempt from two behaviours that exist for stack lifecycle
// commands and are actively wrong here:
//
//   - The source-repo guard. Refusing to run inside a checkout is right for
//     `nself start`; for `nself ci` the checkout IS the subject.
//   - The monorepo redirect. PersistentPreRunE chdirs into the detected
//     backend/ before RunE, so `nself ci` resolved "." — and any relative
//     [repo-root] argument — against backend/ instead of the repo. From
//     inside ntask it announced "Using backend as project root" and failed,
//     because backend/ has no manifest while the repo root has package.json
//     and pnpm-workspace.yaml. It silently overrode an explicitly passed
//     path, which is the part that makes it a correctness bug rather than a
//     convenience one.
//
// Keeping this separate from isSourceSafeCommand records WHY each name is
// exempt, so a future addition to either list does not silently inherit the
// other's semantics.
func isRepoScopedCommand(name string) bool {
	return map[string]bool{
		"ci": true,
	}[name]
}

// isRepoScopedInvocation preserves relative project paths for secret-envelope
// commands, including subcommands of recipients, without exempting other
// commands that happen to share a leaf name.
func isRepoScopedInvocation(cmd *cobra.Command) bool {
	if isRepoScopedCommand(cmd.Name()) {
		return true
	}
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() != "resolve" && c.Name() != "recipients" {
			continue
		}
		parent := c.Parent()
		if parent != nil && parent.Name() == "secrets" && parent.Parent() != nil && parent.Parent().Name() == "config" {
			return true
		}
	}
	return false
}
