package commands

// Purpose: report whether this CLI meets a project's declared minimum version.
// Inputs: project directory, running CLI version, verbose flag.
// Outputs: zero or one doctor result. Constraints: no network or project writes.
// SPORT: cap:cli.cli-min-version.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/version"
)

func checkDoctorCompat(projectDir string, verbose bool) []doctorCheckResult {
	return checkDoctorCompatVersion(projectDir, version.GetVersion(), verbose)
}

// checkDoctorCompatVersion allows deterministic release and development checks.
func checkDoctorCompatVersion(projectDir, running string, verbose bool) []doctorCheckResult {
	manifest, err := build.LoadProjectManifest(projectDir)
	if manifest == nil && err == nil || manifest != nil && manifest.CLIMinVersion == "" {
		return nil
	}
	name := "CLI minimum version"
	result := doctorCheckResult{Name: name}
	if err != nil {
		result.Status, result.Message = "warn", fmt.Sprintf("cannot read project CLI requirement: %v", err)
	} else if !build.ValidateCLIMinVersion(manifest.CLIMinVersion) {
		result.Status, result.Message = "warn", fmt.Sprintf("[E061] invalid cli_min_version %q; Fix: set a semantic version", manifest.CLIMinVersion)
	} else if isDevelopmentCLI(running) || !build.ValidateCLIMinVersion(running) {
		result.Status, result.Message = "skip", fmt.Sprintf("development build (%s): CLI version comparison skipped", running)
	} else if compareCLISemver(running, manifest.CLIMinVersion) < 0 {
		result.Status, result.Message = "warn", fmt.Sprintf("[E060] project needs nself ≥ %s (running %s); Fix: nself update", manifest.CLIMinVersion, running)
	} else {
		result.Status, result.Message = "pass", fmt.Sprintf("nself %s meets project minimum %s", running, manifest.CLIMinVersion)
	}
	printCheck(result.Status, result.Name, result.Message, verbose)
	return []doctorCheckResult{result}
}

var pseudoCLIVersion = regexp.MustCompile(`^v?0\.0\.0-[0-9]{8,14}-[0-9a-f]{6,}$`)

func isDevelopmentCLI(v string) bool {
	return strings.Contains(strings.ToLower(v), "dev") || pseudoCLIVersion.MatchString(v)
}

// compareCLISemver uses the shared numeric comparator and resolves prerelease ties.
func compareCLISemver(a, b string) int {
	a = strings.TrimPrefix(strings.SplitN(strings.TrimPrefix(a, "v"), "+", 2)[0], "v")
	b = strings.TrimPrefix(strings.SplitN(strings.TrimPrefix(b, "v"), "+", 2)[0], "v")
	aParts, bParts := strings.SplitN(a, "-", 2), strings.SplitN(b, "-", 2)
	if order := plugin.CompareVersions(aParts[0], bParts[0]); order != 0 {
		return order
	}
	if len(aParts) == 1 && len(bParts) == 1 {
		return 0
	}
	if len(aParts) == 1 {
		return 1
	}
	if len(bParts) == 1 {
		return -1
	}
	aIDs, bIDs := strings.Split(aParts[1], "."), strings.Split(bParts[1], ".")
	for i := 0; i < len(aIDs) && i < len(bIDs); i++ {
		ai, ae := strconv.ParseUint(aIDs[i], 10, 64)
		bi, be := strconv.ParseUint(bIDs[i], 10, 64)
		if ae == nil && be == nil {
			if ai < bi {
				return -1
			}
			if ai > bi {
				return 1
			}
			continue
		}
		if ae == nil {
			return -1
		}
		if be == nil {
			return 1
		}
		if aIDs[i] < bIDs[i] {
			return -1
		}
		if aIDs[i] > bIDs[i] {
			return 1
		}
	}
	if len(aIDs) < len(bIDs) {
		return -1
	}
	if len(aIDs) > len(bIDs) {
		return 1
	}
	return 0
}
