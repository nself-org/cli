package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// Purpose: path/volume validation helpers for CS_N custom-service env vars,
// split out of custom_services.go to keep that file's parse loop readable.
// Shared by CS_N_PATH, CS_N_ENV_FILE (both single relative paths) and
// CS_N_VOLUMES (comma-separated host:container[:mode] triples whose host
// half is checked the same way). Extracted once a third caller needed the
// same traversal check (G-013), per the repo's DRY-on-third-copy convention.
// Inputs: raw string values read directly from os.Getenv by the caller.
// Outputs: nil on a safe value, otherwise an error naming the problem —
// callers wrap it with the specific CS_N_* var name for context.
// Constraints: intentionally permissive on everything except escaping the
// project root — these are operator-authored env vars, not untrusted input,
// so the goal is catching mistakes, not adversarial hardening.

// validateRelativePath rejects an absolute path or one containing a ".."
// path-traversal segment. Used for any CS_N_* value that names a location
// inside the project tree (CS_N_PATH, CS_N_ENV_FILE).
func validateRelativePath(p string) error {
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("must be a relative path, got %q", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("must not contain '..', got %q", p)
		}
	}
	return nil
}

// validateCustomServiceVolumes checks a CS_N_VOLUMES value: a comma-separated
// list of "host:container[:mode]" entries. Each entry must have at least a
// host and container path; a relative host path (one not starting with "/"
// and not a bare named-volume identifier containing no "/") is checked for
// traversal via validateRelativePath. Named Docker volumes (e.g.
// "my_data:/data") and absolute bind mounts (e.g. "/srv/x:/data") are left to
// the operator's judgment, matching Docker Compose's own permissive stance.
func validateCustomServiceVolumes(raw string) error {
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return fmt.Errorf("contains an empty entry in %q", raw)
		}
		parts := strings.Split(entry, ":")
		if len(parts) < 2 {
			return fmt.Errorf("entry %q must be host:container[:mode]", entry)
		}
		host := parts[0]
		if host == "" {
			return fmt.Errorf("entry %q is missing a host path", entry)
		}
		// Only check paths that look like a project-relative bind mount
		// ("./x", "../x", or a bare relative segment containing "/").
		// Absolute paths and bare named-volume names (no "/") are exempt.
		if !strings.HasPrefix(host, "/") && strings.Contains(host, "/") {
			if err := validateRelativePath(host); err != nil {
				return fmt.Errorf("entry %q: %w", entry, err)
			}
		}
	}
	return nil
}

// validateDockerfilePath checks a CS_N_DOCKERFILE value: relative to the build
// context, no "..", not absolute, no control characters. Symlinks are checked
// at compose time, where the context directory is known.
func validateDockerfilePath(p string) error {
	if err := validateRelativePath(p); err != nil {
		return err
	}
	if strings.TrimSpace(p) != p || strings.ContainsAny(p, "\x00\n\r\t\\") {
		return fmt.Errorf("must be a plain relative path, got %q", p)
	}
	return nil
}

// validateBuildContextPath is the syntax check for CS_N_PATH. It accepts a
// project-relative path (today's rule) or a chain of ".." segments naming an
// ancestor of the project (a monorepo root). A path that climbs and then
// descends ("../sibling") is not an ancestor and is rejected. How far the
// ancestor may reach, and what it must contain, is ValidateBuildContext.
func validateBuildContextPath(p string) error {
	if err := validateRelativePath(p); err == nil {
		return nil
	} else if strings.HasPrefix(p, "/") {
		return err
	}
	for _, seg := range strings.Split(p, "/") {
		if seg != ".." && seg != "." && seg != "" {
			return fmt.Errorf("may name an ancestor of the project ('..' segments only) or a path inside it, got %q", p)
		}
	}
	return nil
}

// ValidateBuildContext enforces E528 on an ancestor CS_N_PATH (key names the
// variable in the message). projectDir is the project root ("" means the
// current directory). A path with no ".." segment is inside the project and
// passes. An ancestor context may reach at most the nearest ancestor of the
// project that holds .git (the project itself when none does), and that
// directory must hold a .dockerignore that excludes .env* and .secrets, so
// the monorepo's secrets are never sent to the image builder.
func ValidateBuildContext(key, projectDir, p string) error {
	if !strings.Contains("/"+p+"/", "/../") {
		return nil
	}
	proj, err := filepath.Abs(projectDir)
	if err != nil {
		return errs.Newf("E528", "%s: cannot resolve the project directory: %v", key, err)
	}
	if real, err := filepath.EvalSymlinks(proj); err == nil {
		proj = real
	}
	ctx := filepath.Clean(filepath.Join(proj, p))
	limit := repositoryRootLimit(proj)
	if ctx != limit && !strings.HasPrefix(ctx, limit+string(filepath.Separator)) {
		return errs.Newf("E528", "%s=%s resolves to %s, above the repository root %s", key, p, ctx, limit)
	}
	env, secrets := dockerignoreCoverage(filepath.Join(ctx, ".dockerignore"))
	if !env || !secrets {
		return errs.Newf("E528", "%s=%s resolves to %s, whose .dockerignore must exclude .env* and .secrets (found .env*: %t, .secrets: %t)", key, p, ctx, env, secrets)
	}
	return nil
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

// dockerignoreCoverage reports whether the .dockerignore at path excludes
// .env* and .secrets. A later negation of the same pattern ("!.env*") cancels
// the exclusion; re-including one named file ("!.env.example") does not. A missing file covers neither.
func dockerignoreCoverage(path string) (env, secrets bool) {
	f, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		neg := strings.HasPrefix(line, "!")
		line = strings.TrimPrefix(strings.TrimPrefix(line, "!"), "/")
		line = strings.TrimPrefix(line, "**/")
		line = strings.TrimSuffix(strings.TrimSuffix(line, "/**"), "/")
		switch line {
		case ".env*":
			env = !neg
		case ".secrets", ".secrets*":
			secrets = !neg
		}
	}
	return env, secrets
}
