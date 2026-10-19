package verify

// Purpose: the production Git implementation, a thin read-only wrapper over
//          the git CLI with fixed arguments.
// Inputs:  the project root.
// Outputs: Git for LoadTrust.
// Constraints: no prompts (GIT_TERMINAL_PROMPT=0), each call bounded by ctx,
//              ref names validated before they reach an argument list.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var refRE = regexp.MustCompile(`^origin/[A-Za-z0-9][A-Za-z0-9._/-]*$`)

const fetchSeconds = 60

// GitCLI runs git in Root.
type GitCLI struct{ Root string }

func (g GitCLI) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", g.Root}, args...)...) //nolint:gosec // fixed git verbs; ref names are validated
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// HasOrigin reports whether an origin remote is configured.
func (g GitCLI) HasOrigin(ctx context.Context) bool {
	_, err := g.run(ctx, "remote", "get-url", "origin")
	return err == nil
}

// Fetch refreshes origin.
func (g GitCLI) Fetch(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, fetchSeconds*time.Second)
	defer cancel()
	_, err := g.run(ctx, "fetch", "--quiet", "origin")
	return err
}

// DefaultRef returns origin/<default branch>.
func (g GitCLI) DefaultRef(ctx context.Context) (string, error) {
	// The remote's own answer first: a local origin/HEAD goes stale when the
	// default branch is renamed.
	lsCtx, cancel := context.WithTimeout(ctx, fetchSeconds*time.Second)
	defer cancel()
	if out, err := g.run(lsCtx, "ls-remote", "--symref", "origin", "HEAD"); err == nil {
		if branch, ok := symrefBranch(string(out)); ok && refRE.MatchString("origin/"+branch) {
			return "origin/" + branch, nil
		}
	}
	if out, err := g.run(ctx, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if ref := strings.TrimSpace(string(out)); refRE.MatchString(ref) {
			return ref, nil
		}
	}
	for _, ref := range []string{"origin/main", "origin/master"} {
		if _, err := g.run(ctx, "rev-parse", "--verify", "--quiet", "refs/remotes/"+ref); err == nil {
			return ref, nil
		}
	}
	return "", fmt.Errorf("origin has no default branch ref")
}

// Show returns path as committed at ref.
func (g GitCLI) Show(ctx context.Context, ref, path string) ([]byte, bool, error) {
	if !refRE.MatchString(ref) || strings.HasPrefix(path, "-") {
		return nil, false, fmt.Errorf("refusing ref %q path %q", ref, path)
	}
	listing, err := g.run(ctx, "ls-tree", "--name-only", ref, "--", path)
	if err != nil {
		return nil, false, err
	}
	if len(bytes.TrimSpace(listing)) == 0 {
		return nil, false, nil
	}
	data, err := g.run(ctx, "show", ref+":./"+path) // ./ makes the path relative to Root, not the repository top
	return data, err == nil, err
}

// symrefBranch reads "ref: refs/heads/<branch>\tHEAD" from ls-remote --symref.
func symrefBranch(out string) (string, bool) {
	for _, line := range strings.Split(out, "\n") {
		rest, ok := strings.CutPrefix(line, "ref: refs/heads/")
		if !ok {
			continue
		}
		branch, _, found := strings.Cut(rest, "\t")
		if !found || branch == "" {
			return "", false
		}
		return branch, true
	}
	return "", false
}
