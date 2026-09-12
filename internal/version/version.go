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
	Version   string = "1.3.6"
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
