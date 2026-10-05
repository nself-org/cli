package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Version, Commit, and BuildDate are set via ldflags at build time.
//
//	go build -ldflags "-X nself/internal/version.Version=1.0.0 -X nself/internal/version.Commit=abc123 -X nself/internal/version.BuildDate=2026-01-01"
var (
	Version   string = "1.4.12"
	Commit    string = "unknown"
	BuildDate string = "unknown"
)

// GetVersion returns the build version string.
func GetVersion() string {
	return Version
}

// GetCommit returns the git commit hash.
func GetCommit() string {
	return Commit
}

// GetBuildDate returns the build date.
func GetBuildDate() string {
	return BuildDate
}

// CapDBDryRunSafe is advertised by builds whose `db migrate up --dry-run`
// (with or without --migration-dir) issues only read statements and applies
// nothing (P7-PROD-77, P7-PROD-84). A caller that forwards --dry-run to a
// remote nself must see it in that remote's `nself version --json` first: the
// version number cannot prove it, because source builds and the released
// v1.4.12 both report 1.4.12 and v1.4.12 applies on a dry-run.
const CapDBDryRunSafe = "db-dry-run-safe"

// Capabilities lists the capability names this build advertises in
// `nself version --json`. Add a name only with the change that earns it.
func Capabilities() []string {
	return []string{CapDBDryRunSafe}
}

// NextMinor returns the next minor release version (patch reset to 0) for a
// "X.Y.Z" version string, e.g. "1.2.7" -> "1.3.0". Used for escape-hatch
// removal-version messaging such as NSELF_LEGACY_ENV_ORDER (CLI-R18), which
// is honored for exactly one minor version. Returns v unchanged if it cannot
// be parsed as three dot-separated integers.
func NextMinor(v string) string {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return v
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	if errMajor != nil || errMinor != nil {
		return v
	}
	return fmt.Sprintf("%d.%d.0", major, minor+1)
}
