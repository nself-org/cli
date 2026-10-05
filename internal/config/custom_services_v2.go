package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// Purpose: the CS_N v2 keys (P7-ADOPT-01): CS_N_DEPENDS_ON, CS_N_NETWORKS,
// CS_N_DOCKERFILE, CS_N_BUILD_TARGET, CS_N_COMMAND and the comma-separated
// CS_N_ENV_FILE list. They give a custom service what twelve hand-written
// compose fragments used to supply, so the generated compose stays the only
// compose (Constitution §1.1).
// Inputs: the CS_<N>_* environment variables, read with os.Getenv.
// Outputs: fields on CustomService; an error naming the key on a bad value.
// Constraints: syntax only. Dependency names are resolved against the whole
// project in build post-validation (after the plugin step), and the network
// prefix is checked at compose time, where the project name is known. Every
// key is additive: a service that sets none renders as before.

var (
	// csDepNameRe is a compose service name as written in CS_N_DEPENDS_ON.
	csDepNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	// csNetworkSyntaxRe is the syntax of a CS_N_NETWORKS entry; the
	// <PROJECT_NAME>_ prefix is enforced at compose time.
	csNetworkSyntaxRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	// csBuildTargetRe is the allowed CS_N_BUILD_TARGET shape.
	csBuildTargetRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
)

// csDependencyConditions maps the short CS_N_DEPENDS_ON condition to the
// compose value. The default (no suffix) is healthy.
var csDependencyConditions = map[string]string{
	"started":   "service_started",
	"healthy":   "service_healthy",
	"completed": "service_completed_successfully",
}

// CustomServicesFromEnv parses CS_1..CS_10 from the process environment. Build
// post-validation uses it to read each service's CS_N_DEPENDS_ON after the
// plugin step, without reloading the project config.
func CustomServicesFromEnv() ([]CustomService, error) {
	return parseCustomServices()
}

// parseCustomServiceV2 reads the v2 keys for service i into cs. Called once
// from parseCustomServices after the existing overrides.
func parseCustomServiceV2(cs *CustomService, i int) error {
	key := func(s string) string { return fmt.Sprintf("CS_%d_%s", i, s) }

	if raw := os.Getenv(key("DEPENDS_ON")); raw != "" {
		deps, err := parseCSDependsOn(key("DEPENDS_ON"), raw)
		if err != nil {
			return err
		}
		cs.DependsOn = deps
	}
	if raw := os.Getenv(key("NETWORKS")); raw != "" {
		nets, err := parseCSNetworks(key("NETWORKS"), raw)
		if err != nil {
			return err
		}
		cs.Networks = nets
	}
	if raw := os.Getenv(key("DOCKERFILE")); raw != "" {
		if err := validateDockerfilePath(raw); err != nil {
			return fmt.Errorf("%s %w", key("DOCKERFILE"), err)
		}
		cs.Dockerfile = raw
	}
	if raw := os.Getenv(key("BUILD_TARGET")); raw != "" {
		if !csBuildTargetRe.MatchString(raw) {
			return fmt.Errorf("%s must match %s, got %q", key("BUILD_TARGET"), csBuildTargetRe, raw)
		}
		cs.BuildTarget = raw
	}
	if raw := os.Getenv(key("COMMAND")); raw != "" {
		// Exec form: split on whitespace, no shell, no quoting.
		cs.Command = strings.Fields(raw)
	}
	if cs.Image != "" && (cs.Dockerfile != "" || cs.BuildTarget != "") {
		return fmt.Errorf("%s and %s only apply to a build: they cannot be combined with %s",
			key("DOCKERFILE"), key("BUILD_TARGET"), key("IMAGE"))
	}
	return nil
}

// parseCSDependsOn splits "name[:started|healthy|completed],..." into
// dependencies. Names are syntax-checked only (E500); existence is checked in
// build post-validation.
func parseCSDependsOn(key, raw string) ([]CSDependency, error) {
	var out []CSDependency
	seen := map[string]bool{}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		name, cond, hasCond := strings.Cut(entry, ":")
		name = strings.TrimSpace(name)
		if !csDepNameRe.MatchString(name) {
			return nil, errs.Newf("E500", "%s has an invalid dependency name %q in %q (use name[:started|healthy|completed])", key, name, raw)
		}
		compose := csDependencyConditions["healthy"]
		if hasCond {
			c, ok := csDependencyConditions[strings.TrimSpace(cond)]
			if !ok {
				return nil, errs.Newf("E500", "%s has an invalid condition %q for %q (use started, healthy or completed)", key, cond, name)
			}
			compose = c
		}
		if seen[name] {
			return nil, errs.Newf("E500", "%s names %q more than once", key, name)
		}
		seen[name] = true
		out = append(out, CSDependency{Name: name, Condition: compose})
	}
	return out, nil
}

// parseCSNetworks splits a comma-separated network list, rejecting empty or
// malformed entries (E501). The <PROJECT_NAME>_ prefix is checked at compose
// time.
func parseCSNetworks(key, raw string) ([]string, error) {
	var out []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if !csNetworkSyntaxRe.MatchString(entry) {
			return nil, errs.Newf("E501", "%s has an invalid network name %q in %q", key, entry, raw)
		}
		out = append(out, entry)
	}
	return out, nil
}

// parseCustomServiceEnvFiles splits the CS_N_ENV_FILE value on commas. Each
// entry must be a relative path without "..". The caller wraps an error with
// the key name.
func parseCustomServiceEnvFiles(raw string) ([]string, error) {
	var out []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, fmt.Errorf("contains an empty entry in %q", raw)
		}
		if err := validateRelativePath(entry); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

// FindDependsCycle returns the first dependency cycle reachable from roots in
// edges (name -> dependencies) as "a -> b -> a", or nil. Each node is visited
// once (depth-first, memoised), so a diamond is not a cycle and a cycle among
// services reached through a root is reported even when the root is not on it.
// Shared by compose generation (custom services only) and build
// post-validation (the whole compose set).
func FindDependsCycle(edges map[string][]string, roots []string) []string {
	const grey, black = 1, 2
	state := map[string]int{}
	var path []string
	var visit func(n string) []string
	visit = func(n string) []string {
		state[n] = grey
		path = append(path, n)
		for _, next := range edges[n] {
			switch state[next] {
			case grey:
				i := 0
				for path[i] != next {
					i++
				}
				return append(append([]string(nil), path[i:]...), next)
			case 0:
				if c := visit(next); c != nil {
					return c
				}
			}
		}
		path = path[:len(path)-1]
		state[n] = black
		return nil
	}
	for _, r := range roots {
		if state[r] == 0 {
			if c := visit(r); c != nil {
				return c
			}
		}
	}
	return nil
}
