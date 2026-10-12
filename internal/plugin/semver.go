package plugin

// Purpose: the one semver comparison used by the plugin installer, the bundle
// resolver and the version helpers (D-0191). Before this file the bundle package
// and deps.go each carried their own parser and disagreed on "v1.2.3" and on
// pre-release tags.
// Inputs: two version strings: optional leading "v", surrounding space allowed,
// MAJOR[.MINOR[.PATCH]], optional "-prerelease" suffix.
// Outputs: -1, 0 or 1, or an error when either side is not a version.
// Constraints: no I/O, no external dependency. Build metadata ("+...") is
// ignored, as semver requires: "1.2.3+b1" equals "1.2.3" (manifest.go accepts
// it in installed versions). Pre-release labels are compared as plain strings
// (not by semver identifier rules); a pre-release sorts below its release.

import (
	"fmt"
	"strconv"
	"strings"
)

// semver holds the parsed components of a version string.
type semver struct {
	major, minor, patch int
	pre                 string // pre-release label after "-", e.g. "alpha.1"
}

// parseSemver parses s as [v]MAJOR[.MINOR[.PATCH]][-pre][+build]; the build
// part is dropped. Missing numeric
// components are zero, so "1.2" parses the same as "1.2.0".
func parseSemver(s string) (semver, error) {
	raw := s
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if idx := strings.IndexByte(s, '+'); idx >= 0 {
		s = s[:idx] // build metadata never affects order
	}
	if s == "" {
		return semver{}, fmt.Errorf("empty version string")
	}
	pre := ""
	if idx := strings.IndexByte(s, '-'); idx >= 0 {
		pre = s[idx+1:]
		s = s[:idx]
	}
	var nums [3]int
	for i, p := range strings.SplitN(s, ".", 3) {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, fmt.Errorf("non-numeric component %q in %q", p, raw)
		}
		nums[i] = n
	}
	return semver{major: nums[0], minor: nums[1], patch: nums[2], pre: pre}, nil
}

// CompareSemver returns -1 if a < b, 0 if a == b and 1 if a > b. It returns an
// error (and 0) when either version cannot be parsed.
//
//	CompareSemver("1.10.0", "1.9.9") → 1, nil
//	CompareSemver("1.2.3-beta", "1.2.3") → -1, nil
func CompareSemver(a, b string) (int, error) {
	pa, err := parseSemver(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseSemver(b)
	if err != nil {
		return 0, err
	}
	for _, d := range [3]int{pa.major - pb.major, pa.minor - pb.minor, pa.patch - pb.patch} {
		if d != 0 {
			return sign(d), nil
		}
	}
	switch {
	case pa.pre == pb.pre:
		return 0, nil
	case pa.pre == "":
		return 1, nil
	case pb.pre == "":
		return -1, nil
	default:
		return strings.Compare(pa.pre, pb.pre), nil
	}
}

// sign normalises n to -1, 0 or 1.
func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}
