package config

import (
	"bufio"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
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
// current directory); dockerfile is CS_N_DOCKERFILE ("" means Dockerfile). A
// path with no ".." segment is inside the project and passes. An ancestor
// context may reach at most the nearest ancestor of the project that holds
// .git (the project itself when none does), and the ignore file BuildKit will
// use must exclude the .env* and .secrets files at the context root, in every
// directory down to the project, and below it, so the monorepo's secrets are
// never sent to the image builder. A bare ".env*" is not enough: Docker
// matches it at the context root only, so proj/.env would still be sent;
// "**/.env*" and "**/.secrets" are the patterns that cover nested paths.
func ValidateBuildContext(key, projectDir, p, dockerfile string) error {
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
	rel, err := filepath.Rel(ctx, proj)
	if err != nil {
		return errs.Newf("E528", "%s: cannot relate %s to %s: %v", key, proj, ctx, err)
	}
	file := dockerignoreFile(ctx, dockerfile)
	pats, ok := readDockerignore(file)
	if !ok {
		return errs.Newf("E528", "%s=%s resolves to %s, which has no %s: an ancestor context needs one that excludes **/.env* and **/.secrets", key, p, ctx, filepath.Base(file))
	}
	if miss := uncoveredSecretPath(pats, filepath.ToSlash(rel)); miss != "" {
		return errs.Newf("E528", "%s=%s resolves to %s, whose %s does not exclude %s (a bare .env* matches only the context root; use **/.env* and **/.secrets)", key, p, ctx, filepath.Base(file), miss)
	}
	return nil
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

// ignorePattern is one compiled .dockerignore line.
type ignorePattern struct {
	re  *regexp.Regexp
	neg bool
}

// readDockerignore parses an ignore file the way Docker does (comments and
// blanks skipped, "!" negates, patterns cleaned, leading "/" dropped). The bool
// is false when the file cannot be read.
func readDockerignore(path string) ([]ignorePattern, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	var out []ignorePattern
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		neg := strings.HasPrefix(line, "!")
		line = strings.TrimSpace(strings.TrimPrefix(line, "!"))
		if line == "" {
			continue
		}
		line = pathpkg.Clean(line)
		if len(line) > 1 && line[0] == '/' {
			line = line[1:]
		}
		if re, err := compileIgnore(line); err == nil {
			out = append(out, ignorePattern{re: re, neg: neg})
		}
	}
	return out, true
}

// compileIgnore turns one pattern into an anchored regexp with Docker's
// rules: "*" is any run of non-"/" characters, "?" one such character, "**/"
// any number of directories (including none), a trailing "**" anything.
func compileIgnore(pat string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pat); i++ {
		switch c := pat[i]; c {
		case '*':
			if i+1 < len(pat) && pat[i+1] == '*' {
				for i+1 < len(pat) && pat[i+1] == '*' {
					i++
				}
				if i+1 < len(pat) && pat[i+1] == '/' {
					i++
					b.WriteString("(.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '[', ']':
			b.WriteByte(c)
		case '\\':
			if i+1 < len(pat) {
				i++
				b.WriteString(regexp.QuoteMeta(string(pat[i])))
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// dockerignored reports whether the context-relative slash path rel is
// excluded: the last pattern that matches rel or one of its parent
// directories decides, a "!" pattern re-including.
func dockerignored(pats []ignorePattern, rel string) bool {
	excluded := false
	for _, p := range pats {
		m := p.re.MatchString(rel)
		for parent := pathpkg.Dir(rel); !m && parent != "." && parent != "/"; parent = pathpkg.Dir(parent) {
			m = p.re.MatchString(parent)
		}
		if m {
			excluded = !p.neg
		}
	}
	return excluded
}

// uncoveredSecretPath returns the first secret-looking path (relative to the
// context) that the patterns would not exclude, or "". It probes .env files
// and .secrets in the context root, in every directory down to the project
// (relProj, slash form), and in a directory below the project.
func uncoveredSecretPath(pats []ignorePattern, relProj string) string {
	dirs := []string{""}
	if relProj != "" && relProj != "." {
		acc := ""
		for _, seg := range strings.Split(relProj, "/") {
			acc = pathpkg.Join(acc, seg)
			dirs = append(dirs, acc)
		}
		dirs = append(dirs, pathpkg.Join(relProj, "nested", "deep"))
	} else {
		dirs = append(dirs, "nested/deep")
	}
	for _, d := range dirs {
		for _, name := range []string{".env", ".env.local", ".env.secrets", ".secrets", ".secrets/token"} {
			path := pathpkg.Join(d, name)
			if !dockerignored(pats, path) {
				return path
			}
		}
	}
	return ""
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
