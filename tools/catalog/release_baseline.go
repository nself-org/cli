package main

// Purpose: prevent -check from accepting a released plugin's silent delisting.
// Inputs: the release input path and its current rows; the input repo's main ref.
// Outputs: removed-row problems or an error when a Git baseline cannot be read.
// Constraints: read-only, no fetch; standalone inputs have no Git baseline.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/tools/catalog/model"
)

func removedReleaseRows(path string, current *model.Releases) ([]string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	root, err := releaseRepoRoot(filepath.Dir(abs))
	if err != nil || root == "" {
		return nil, err
	}
	base, err := mainReleaseRows(root, abs)
	if err != nil {
		return nil, err
	}
	var problems []string
	for _, slug := range sortedKeys(base) {
		if _, ok := current.Releases[slug]; !ok {
			problems = append(problems, slug+": release row present on main was removed")
		}
	}
	return problems, nil
}

// releaseRepoRoot finds the input repository, including linked worktrees.
func releaseRepoRoot(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("release baseline: %w", err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func mainReleaseRows(root, path string) (map[string]model.Release, error) {
	ref, err := releaseMainRef(root)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)
	listed, err := releaseGit(root, "ls-tree", "--name-only", ref, "--", rel)
	if err != nil || len(listed) == 0 { // A new releases file has no prior rows.
		return nil, err
	}
	data, err := releaseGit(root, "show", ref+":"+rel)
	if err != nil {
		return nil, err
	}
	var base model.Releases
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("release baseline %s:%s: %w", ref, rel, err)
	}
	if base.SchemaVersion != model.ReleasesVersion {
		return nil, fmt.Errorf("release baseline %s:%s: unsupported schema_version %d", ref, rel, base.SchemaVersion)
	}
	return base.Releases, nil
}

func releaseMainRef(root string) (string, error) {
	for _, ref := range []string{"refs/remotes/origin/main", "refs/heads/main"} {
		if _, err := releaseGit(root, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err == nil {
			return ref, nil
		} else {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 {
				return "", err
			}
		}
	}
	return "", fmt.Errorf("release baseline: -check needs origin/main or main in %s; fetch origin main first", root)
}

// releaseGit bounds local Git reads; failures retain the command diagnostic.
func releaseGit(root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("release baseline git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
