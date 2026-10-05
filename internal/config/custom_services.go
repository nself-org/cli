package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ParseCustomServices parses CS_1 through CS_10 environment variables into
// CustomService structs. Each CS_N value uses the format:
//
//	name:template[:port][:route]
//
// If port is omitted or zero, it auto-assigns 8000+N.
// Per-service overrides are read from CS_N_PUBLIC, CS_N_MEMORY, CS_N_CPU,
// CS_N_PORT, CS_N_ROUTE, CS_N_HEALTHCHECK, CS_N_ENV_PASSTHROUGH,
// CS_N_IMAGE, CS_N_ENV_FILE, and CS_N_VOLUMES environment variables, plus the
// v2 keys (CS_N_DEPENDS_ON, CS_N_NETWORKS, CS_N_DOCKERFILE, CS_N_BUILD_TARGET,
// CS_N_COMMAND; see custom_services_v2.go).
func parseCustomServices() ([]CustomService, error) {
	var services []CustomService
	for i := 1; i <= 10; i++ {
		raw := os.Getenv(fmt.Sprintf("CS_%d", i))
		if raw == "" {
			continue
		}

		// Format: name:template[:port][:route]
		parts := strings.SplitN(raw, ":", 4)
		if len(parts) < 2 {
			return nil, fmt.Errorf("CS_%d has invalid format: expected name:template[:port][:route], got %q", i, raw)
		}

		cs := CustomService{
			Index:    i,
			Name:     parts[0],
			Template: parts[1],
		}

		if err := validateCustomServiceName(cs.Name); err != nil {
			return nil, fmt.Errorf("CS_%d: %w", i, err)
		}
		// Preserve the as-configured name before sanitization so callers that
		// need to resolve an on-disk path (e.g. the compose generator's
		// default `./services/<name>` build context) can find a directory
		// named "ping_api" even though the sanitized Docker service name is
		// "ping-api". See CustomService.RawName for the full rationale.
		cs.RawName = cs.Name
		sanitized, err := SanitizeName(cs.Name)
		if err != nil {
			return nil, fmt.Errorf("CS_%d has invalid name %q: %w", i, cs.Name, err)
		}
		cs.Name = sanitized

		// Optional port
		if len(parts) >= 3 && parts[2] != "" {
			p, err := strconv.Atoi(parts[2])
			if err != nil {
				return nil, fmt.Errorf("CS_%d has invalid format: expected numeric port, got %q", i, parts[2])
			}
			cs.Port = p
		}
		if cs.Port == 0 {
			cs.Port = 8000 + i // auto-assign
		}

		// Optional route
		if len(parts) >= 4 && parts[3] != "" {
			cs.Route = parts[3]
		}

		// Additional per-service config
		cs.Public = getEnvBool(fmt.Sprintf("CS_%d_PUBLIC", i), cs.Route != "")
		cs.Memory = getEnvOr(fmt.Sprintf("CS_%d_MEMORY", i), "256m")
		cs.CPU = getEnvOr(fmt.Sprintf("CS_%d_CPU", i), "0.5")
		cs.TablePrefix = os.Getenv(fmt.Sprintf("CS_%d_TABLE_PREFIX", i))
		cs.ExtraEnv = os.Getenv(fmt.Sprintf("CS_%d_ENV", i))
		cs.HealthCheck = os.Getenv(fmt.Sprintf("CS_%d_HEALTHCHECK", i))
		cs.EnvPassthrough = os.Getenv(fmt.Sprintf("CS_%d_ENV_PASSTHROUGH", i))

		// Optional build context path override. Rejects absolute paths and
		// path traversal so a misconfigured env can't escape the project root;
		// the one exception is an ancestor ('..' only, e.g. a monorepo root),
		// bounded by ValidateBuildContext at compose time (E528).
		if p := os.Getenv(fmt.Sprintf("CS_%d_PATH", i)); p != "" {
			if err := validateBuildContextPath(p); err != nil {
				return nil, fmt.Errorf("CS_%d_PATH %w", i, err)
			}
			cs.BuildPath = p
		}

		// CS_N_IMAGE: run a pre-built (optionally digest-pinned) image instead
		// of building from a Dockerfile. Mutually exclusive with CS_N_PATH,
		// which only makes sense for the build path (G-013).
		cs.Image = os.Getenv(fmt.Sprintf("CS_%d_IMAGE", i))
		if cs.Image != "" && cs.BuildPath != "" {
			return nil, fmt.Errorf("CS_%d_IMAGE and CS_%d_PATH are mutually exclusive: a service either builds from a Dockerfile (CS_%d_PATH) or runs a pre-built image (CS_%d_IMAGE), not both", i, i, i, i)
		}

		// CS_N_ENV_FILE: dotenv-format file of extra env vars, injected at
		// build time (see coreEnvVars). Same relative-path rules as CS_N_PATH.
		// A comma-separated list loads in order, later files winning (v2).
		if p := os.Getenv(fmt.Sprintf("CS_%d_ENV_FILE", i)); p != "" {
			files, err := parseCustomServiceEnvFiles(p)
			if err != nil {
				return nil, fmt.Errorf("CS_%d_ENV_FILE %w", i, err)
			}
			cs.EnvFile = files[0]
			cs.EnvFiles = files
		}

		// CS_N_VOLUMES: comma-separated "host:container[:mode]" bind mounts,
		// appended to the generated service. Each relative host path is
		// subject to the same traversal check as CS_N_PATH.
		if v := os.Getenv(fmt.Sprintf("CS_%d_VOLUMES", i)); v != "" {
			if err := validateCustomServiceVolumes(v); err != nil {
				return nil, fmt.Errorf("CS_%d_VOLUMES %w", i, err)
			}
			cs.Volumes = v
		}

		// Override port/route if explicitly set
		if p := getEnvInt(fmt.Sprintf("CS_%d_PORT", i), 0); p != 0 {
			cs.Port = p
		}
		if r := os.Getenv(fmt.Sprintf("CS_%d_ROUTE", i)); r != "" {
			cs.Route = r
		}

		// CS_N v2 keys: DEPENDS_ON, NETWORKS, DOCKERFILE, BUILD_TARGET, COMMAND.
		if err := parseCustomServiceV2(&cs, i); err != nil {
			return nil, err
		}

		services = append(services, cs)
	}
	return services, nil
}

// dockerignoreFile returns the ignore file BuildKit uses for this build: a
// "<dockerfile>.dockerignore" beside the Dockerfile replaces .dockerignore.
func dockerignoreFile(ctx, dockerfile string) string {
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	specific := filepath.Join(ctx, filepath.FromSlash(dockerfile)) + ".dockerignore"
	if _, err := os.Stat(specific); err == nil {
		return specific
	}
	return filepath.Join(ctx, ".dockerignore")
}

// repositoryRootLimit returns the nearest directory at or above proj that
// holds .git (a directory, or a file for a linked worktree); proj itself when
// none does.
func repositoryRootLimit(proj string) string {
	for d := proj; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return proj
		}
	}
}

// Build-context secret guard, part 2 (E528): the dockerignore helpers that
// custom_services_validate.go uses, kept here to stay under the file-size
// ratchet.

// safeReincludeSuffixes are the names a "!" pattern may re-include: example
// files that carry no secret.
var safeReincludeSuffixes = []string{".example", ".sample", ".template", ".dist"}

// reincludedSecret returns the first "!" pattern of pats whose match set can
// cover a .env* or .secrets path, or "". Structural, not a probe list: a
// pattern is refused when any of its segments can spell ".env*" or ".secrets"
// (or is "**"), unless its last segment ends in a safe example suffix. A
// literal pattern naming a directory of ctx re-includes everything under it,
// so it is refused when a secret below it is no longer excluded. Docker
// applies the last matching pattern, so a "!" pattern after an exclusion wins.
func reincludedSecret(pats []ignorePattern, ctx string) string {
	for _, p := range pats {
		if !p.neg {
			continue
		}
		segs := strings.Split(p.text, "/")
		last := segs[len(segs)-1]
		safe := false
		for _, suf := range safeReincludeSuffixes {
			safe = safe || (last != "**" && strings.HasSuffix(last, suf))
		}
		if safe {
			continue
		}
		wild := false
		for _, s := range segs {
			wild = wild || s == "**" || segmentMaySpellSecret(s)
		}
		if wild {
			return p.text
		}
		if fi, err := os.Stat(filepath.Join(ctx, filepath.FromSlash(p.text))); err == nil && fi.IsDir() {
			for _, name := range []string{".env", ".secrets/token"} {
				if !dockerignored(pats, p.text+"/"+name) {
					return p.text
				}
			}
		}
	}
	return ""
}

// segmentMaySpellSecret reports whether one path segment of a pattern can
// match the name ".secrets" or a name starting ".env". A segment with a
// wildcard is judged by its literal prefix; one with none by the name.
func segmentMaySpellSecret(seg string) bool {
	i := strings.IndexAny(seg, "*?[\\")
	if i < 0 {
		return seg == ".secrets" || strings.HasPrefix(seg, ".env")
	}
	lit := seg[:i]
	return strings.HasPrefix(".secrets", lit) || strings.HasPrefix(".env", lit) || strings.HasPrefix(lit, ".env")
}
