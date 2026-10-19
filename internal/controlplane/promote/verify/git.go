package verify

// Purpose: the production Git implementation, a thin read-only wrapper over
//          the git CLI with fixed arguments.
// Inputs:  the project root.
// Outputs: Git for LoadTrust.
// Constraints: no prompts (GIT_TERMINAL_PROMPT=0), no replacement objects; every call is bounded in
//              time and output; the trust commit is found from the remote's own
//              answer and fetched explicitly, then addressed by its commit id so
//              no local tag or stale ref can stand in for it.

import (
	"bytes"
	"context"
	"errors"
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
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-replace-objects", "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "protocol.ext.allow=never", "-C", g.Root}, args...)...) //nolint:gosec // fixed git verbs; branch and commit names are validated
	cmd.Env = append(scrubGitEnv(os.Environ()), "GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitWriter{w: &stdout, left: maxGitOutput}
	cmd.Stderr = &limitWriter{w: &stderr, left: 4096}
	cmd.WaitDelay = subprocessWaitDelay
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// HasOrigin reports whether Root is inside a git work tree with an origin
// remote. Git answering "no" is false; git failing to run at all is an error.
func (g GitCLI) HasOrigin(ctx context.Context) (bool, error) {
	for _, args := range [][]string{{"rev-parse", "--git-dir"}, {"remote", "get-url", "origin"}} {
		if _, err := g.run(ctx, args...); err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				return false, nil
			}
			return false, err
		}
	}
	return true, nil
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

// gitEnvKept are the GIT_ variables that only carry transport settings. Every
// other GIT_ variable (GIT_DIR, GIT_CONFIG_*, GIT_OBJECT_DIRECTORY, ...) could
// point git at a repository or configuration the operator did not choose.
var gitEnvKept = []string{"GIT_SSH=", "GIT_SSH_COMMAND=", "GIT_SSH_VARIANT=", "GIT_ASKPASS=", "GIT_HTTP_", "GIT_SSL_", "GIT_CURL_", "GIT_PROXY_SSL_"}

// scrubGitEnv drops the GIT_ variables that can redirect git.
func scrubGitEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, "GIT_") {
			out = append(out, kv)
			continue
		}
		for _, keep := range gitEnvKept {
			if strings.HasPrefix(kv, keep) {
				out = append(out, kv)
				break
			}
		}
	}
	return out
}
