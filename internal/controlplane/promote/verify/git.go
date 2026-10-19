package verify

// Purpose: the production Git implementation, a thin read-only wrapper over
//          the git CLI with fixed arguments.
// Inputs:  the project root.
// Outputs: Git for LoadTrust.
// Constraints: no prompts (GIT_TERMINAL_PROMPT=0); every call is bounded in
//              time and output; the trust commit is found from the remote's own
//              answer and fetched explicitly, then addressed by its commit id so
//              no local tag or stale ref can stand in for it.

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

var (
	branchRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,200}$`)
	commitRE = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
)

const (
	gitSeconds     = 60
	maxGitOutput   = 4 << 20
	trustFileLimit = 1 << 20
)

// GitCLI runs git in Root.
type GitCLI struct{ Root string }

func (g GitCLI) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitSeconds*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", g.Root}, args...)...) //nolint:gosec // fixed git verbs; branch and commit names are validated
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitWriter{w: &stdout, left: maxGitOutput}
	cmd.Stderr = &limitWriter{w: &stderr, left: 4096}
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// HasOrigin reports whether an origin remote is configured.
func (g GitCLI) HasOrigin(ctx context.Context) bool {
	_, err := g.run(ctx, "remote", "get-url", "origin")
	return err == nil
}

// PinDefault asks origin which branch is its default, fetches exactly that
// branch (whatever the configured fetch refspecs say) and returns the commit id
// it points to. It fails when any step cannot be established.
func (g GitCLI) PinDefault(ctx context.Context) (string, error) {
	out, err := g.run(ctx, "ls-remote", "--symref", "origin", "HEAD")
	if err != nil {
		return "", err
	}
	branch, ok := symrefBranch(string(out))
	if !ok || !branchRE.MatchString(branch) || strings.Contains(branch, "..") || strings.HasSuffix(branch, ".lock") || strings.HasSuffix(branch, "/") {
		return "", fmt.Errorf("origin did not name a usable default branch")
	}
	tracking := "refs/remotes/origin/" + branch
	if _, err := g.run(ctx, "fetch", "--quiet", "--no-tags", "origin", "+refs/heads/"+branch+":"+tracking); err != nil {
		return "", err
	}
	sha, err := g.run(ctx, "rev-parse", "--verify", "--quiet", tracking+"^{commit}")
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(string(sha))
	if !commitRE.MatchString(commit) {
		return "", fmt.Errorf("origin's default branch did not resolve to a commit")
	}
	return commit, nil
}

// Show returns path as committed at the commit id.
func (g GitCLI) Show(ctx context.Context, commit, path string) ([]byte, bool, error) {
	if !commitRE.MatchString(commit) || strings.HasPrefix(path, "-") {
		return nil, false, fmt.Errorf("refusing commit %q path %q", commit, path)
	}
	listing, err := g.run(ctx, "ls-tree", "--name-only", commit, "--", path)
	if err != nil {
		return nil, false, err
	}
	if len(bytes.TrimSpace(listing)) == 0 {
		return nil, false, nil
	}
	data, err := g.run(ctx, "show", commit+":./"+path) // ./ makes the path relative to Root, not the repository top
	if err == nil && len(data) > trustFileLimit {
		return nil, false, fmt.Errorf("%s is larger than %d bytes", path, trustFileLimit)
	}
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
