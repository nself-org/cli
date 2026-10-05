package commands

// Purpose: Resolves and validates the flag/env inputs to `nself start` into a
// typed startOpts struct, and classifies onboarding/error state used before
// and after the main start sequence runs. Split out of start.go (CLI-R12) to
// keep the command's flag-resolution and error-classification logic apart
// from the orchestration body in runStart.
// Inputs: the cobra.Command carrying parsed flags (resolveStartOpts), an
// error returned by the start sequence (classifyStartError), and ambient
// filesystem state under the user's config dir (isFirstStart).
// Outputs: a populated startOpts, a bool for first-run detection, and a
// short categorical string safe to attach to telemetry events.
// Constraints: pure move — no behavior changes. resolveStartOpts must keep
// reading the same flag names and env var fallbacks (NSELF_EMBEDDED_PG,
// NSELF_SKIP_DB_INIT, NSELF_PROFILE) that runStart and its tests depend on.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/oplock"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// startOpts holds the resolved flags for the start command.
type startOpts struct {
	verbose          bool
	debug            bool
	skipHealthChecks bool
	timeout          int
	fresh            bool
	cleanStart       bool
	quick            bool
	skipPortCheck    bool
	skipBuild        bool
	skipPlugins      bool
	watch            bool
	quiet            bool
	embeddedPG       bool
	skipDBInit       bool
	// profile is forwarded to an automatic rebuild when the compose file is
	// stale. It has no effect when --skip-build is set.
	profile compose.ProfileName
}

func resolveStartOpts(cmd *cobra.Command) (startOpts, error) {
	verbose, _ := cmd.Flags().GetBool("verbose")
	debug, _ := cmd.Flags().GetBool("debug")
	skipHealth, _ := cmd.Flags().GetBool("skip-health-checks")
	timeout, _ := cmd.Flags().GetInt("timeout")
	fresh, _ := cmd.Flags().GetBool("fresh")
	forceRecreate, _ := cmd.Flags().GetBool("force-recreate")
	cleanStart, _ := cmd.Flags().GetBool("clean-start")
	quick, _ := cmd.Flags().GetBool("quick")
	skipPortCheck, _ := cmd.Flags().GetBool("skip-port-check")
	skipBuild, _ := cmd.Flags().GetBool("skip-build")
	skipPlugins, _ := cmd.Flags().GetBool("skip-plugins")
	watch, _ := cmd.Flags().GetBool("watch")
	// P7-LIVE-13: a watching start never holds the project operation lock for
	// the life of its loop (EPIC D12). The registry already classes `start
	// --watch` as stream, which takes no lock; this covers a lock taken
	// through the context by any other route. No lock in ctx: a no-op.
	if watch {
		oplock.FromContext(cmd.Context()).Release()
	}
	quiet, _ := cmd.Flags().GetBool("quiet")
	embeddedPG, _ := cmd.Flags().GetBool("embedded-pg")
	// NSELF_EMBEDDED_PG env var is the fallback when the flag is not set.
	if !embeddedPG && os.Getenv("NSELF_EMBEDDED_PG") == "true" {
		embeddedPG = true
	}
	skipDBInit, _ := cmd.Flags().GetBool("skip-db-init")
	// NSELF_SKIP_DB_INIT env var allows CI pipelines to set this without modifying scripts.
	if !skipDBInit && os.Getenv("NSELF_SKIP_DB_INIT") == "true" {
		skipDBInit = true
	}

	// ── Profile resolution ────────────────────────────────────────────
	// Priority: --profile flag > NSELF_PROFILE env var > default ("app").
	profileStr, _ := cmd.Flags().GetString("profile")
	if profileStr == "" {
		profileStr = os.Getenv("NSELF_PROFILE")
	}
	profile := compose.ProfileName(profileStr)
	if _, known := compose.ProfileForName(profile); !known && profileStr != "" {
		ui.Warn(fmt.Sprintf("Unknown profile %q — valid values: %s. Falling back to \"app\".", profileStr, strings.Join(compose.ValidProfiles(), ", ")))
		profile = compose.ProfileApp
	}

	// --force-recreate is an alias for --fresh.
	if forceRecreate {
		fresh = true
	}

	// --quick overrides timeout and required percentage.
	if quick {
		timeout = 30
	}

	// Clamp timeout to valid range.
	if timeout < 30 {
		timeout = 30
	}
	if timeout > 600 {
		timeout = 600
	}

	return startOpts{
		verbose:          verbose,
		debug:            debug,
		skipHealthChecks: skipHealth,
		timeout:          timeout,
		fresh:            fresh,
		cleanStart:       cleanStart,
		quick:            quick,
		skipPortCheck:    skipPortCheck,
		skipBuild:        skipBuild,
		skipPlugins:      skipPlugins,
		watch:            watch,
		quiet:            quiet,
		embeddedPG:       embeddedPG,
		skipDBInit:       skipDBInit,
		profile:          profile,
	}, nil
}

// isFirstStart returns true when ~/.config/nself/onboarding.json does not yet
// contain a "start_completed" entry. It writes the marker on first call.
func isFirstStart() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	markerPath := filepath.Join(home, ".config", "nself", "onboarding.json")
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		// First start: write the marker before returning true.
		if mkErr := os.MkdirAll(filepath.Dir(markerPath), 0o700); mkErr == nil {
			if wErr := os.WriteFile(markerPath, []byte(`{"start_completed":true}`), 0o600); wErr != nil {
				ui.Warn("could not write start marker: " + wErr.Error())
			}
		}
		return true
	}
	return false
}

// classifyStartError maps a start error to a categorical string safe for telemetry.
func classifyStartError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case containsAny(msg, "port", "already in use", "bind: address"):
		return "port-collision"
	case containsAny(msg, "docker", "Docker", "cannot connect to the Docker", "docker not found", "docker info"):
		return "docker-not-running"
	case containsAny(msg, "pull", "image", "manifest", "registry"):
		return "image-pull-failed"
	case containsAny(msg, "health", "timeout", "deadline exceeded", "context deadline"):
		return "healthcheck-timeout"
	default:
		return "other"
	}
}
