package docker

// Compose version floor (D-0548).
//
// Purpose: docker compose before 2.20 (compose-go before v1.16) rejects a
// double-quoted compose.env value that ends in a backslash with "unterminated
// quoted value", so the stack cannot start. This file reads the installed
// compose version and says whether it is new enough.
// Inputs:  a context (ComposeVersion) or a version string (the rest).
// Outputs: the version string, parsed numbers, a verdict, and the message to
//          show when the version is too old.
// Constraints: ComposeVersion is the only function that runs a subprocess.
//              Parsing and the verdict are pure. An unrecognised string is
//              ComposeUnknown, never ComposeTooOld, so a custom build does
//              not block a start.

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// MinComposeVersion is the oldest docker compose that reads every compose.env
// value nSelf writes.
const MinComposeVersion = "2.20.0"

// ComposeVerdict is the outcome of comparing a version with MinComposeVersion.
type ComposeVerdict int

const (
	// ComposeOK means the version is at least MinComposeVersion.
	ComposeOK ComposeVerdict = iota
	// ComposeTooOld means the version is below MinComposeVersion.
	ComposeTooOld
	// ComposeUnknown means the string could not be parsed.
	ComposeUnknown
)

// ComposeVersion returns the output of `docker compose version --short`,
// trimmed. It fails when docker or the compose plugin is missing.
func ComposeVersion(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "compose", "version", "--short").Output()
	if err != nil {
		return "", fmt.Errorf("docker compose version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ParseComposeVersion reads "2.29.7", "v2.29.7", "2.29.7-desktop.1",
// "2.21.0+azure-1" or "2.20" (patch 0). It reports ok=false for anything else.
func ParseComposeVersion(s string) (major, minor, patch int, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, false
	}
	nums := [3]int{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], true
}

// CheckComposeVersion compares s with MinComposeVersion.
func CheckComposeVersion(s string) ComposeVerdict {
	major, minor, _, ok := ParseComposeVersion(s)
	if !ok {
		return ComposeUnknown
	}
	// MinComposeVersion is 2.20.0, so major and minor decide.
	if major < 2 || (major == 2 && minor < 20) {
		return ComposeTooOld
	}
	return ComposeOK
}

// ComposeTooOldMessage explains a too-old compose version and the fix.
func ComposeTooOldMessage(v string) string {
	return fmt.Sprintf(
		"docker compose %s is older than 2.20. Compose before 2.20 rejects a double-quoted compose.env value that ends in a backslash, so the stack can fail to start. Upgrade Docker Desktop or the docker-compose-plugin package to 2.20 or newer.",
		v)
}
