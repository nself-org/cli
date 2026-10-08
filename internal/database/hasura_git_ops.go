package database

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// Purpose: git-integrated Hasura metadata operations — export-and-commit,
// git status of the metadata directory, checkout-and-apply from a ref, the
// track-table SQL/YAML hint generator, and before/after snapshot comparison.
// Inputs: a context, *config.Config, and a project directory (plus a git ref
// or commit message where relevant).
// Outputs: a commit hash, HasuraGitStatus, a YAML hint string, a snapshot
// []byte, or a human-readable diff string.
// Constraints: split out of hasura_git.go (CLI-R12) as a pure move; no
// behavior changed. Depends on MetadataDrift/HasuraGitStatus/
// extractTablesFromMetadata in hasura_git.go, and HasuraExportToYAML /
// HasuraApplyMetadata / postMetadata / metadataRequest from the other hasura
// files in this package.

// ExportAndCommitMetadata exports Hasura metadata and creates a git commit.
// commitMsg is the commit message; if empty, a default message is generated.
// Returns the commit hash on success.
func ExportAndCommitMetadata(ctx context.Context, cfg *config.Config, projectDir string, commitMsg string) (string, error) {
	if _, err := HasuraExportToYAML(ctx, cfg, projectDir); err != nil {
		return "", fmt.Errorf("export metadata to YAML: %w", err)
	}

	return CommitMetadata(ctx, projectDir, commitMsg)
}

// CommitMetadata commits metadata already written by an export or remote sync.
func CommitMetadata(ctx context.Context, projectDir, commitMsg string) (string, error) {
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("chore(hasura): export metadata %s", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	}

	addCmd := exec.CommandContext(ctx, "git", "-C", projectDir, "add", "hasura/metadata/")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add hasura/metadata/: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	commitCmd := exec.CommandContext(ctx, "git", "-C", projectDir, "commit", "-m", commitMsg)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	hashOut, err := exec.CommandContext(ctx, "git", "-C", projectDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(hashOut)), nil
}

// ArchiveMetadataRef obtains only the metadata tree without changing the checkout.
func ArchiveMetadataRef(ctx context.Context, projectDir, ref string) ([]byte, error) {
	// A leading dash is rejected even though -- terminates options: git archive
	// interprets the tree-ish independently of its path arguments.
	if ref == "" || strings.HasPrefix(ref, "-") {
		return nil, fmt.Errorf("invalid git ref %q", ref)
	}
	var out, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "-C", projectDir, "archive", "--format=tar", "--prefix=metadata/", ref+":hasura/metadata")
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("archive metadata ref %q: %w: %s", ref, err, strings.TrimSpace(stderr.String()))
	}
	var normalized bytes.Buffer
	r, w := tar.NewReader(&out), tar.NewWriter(&normalized)
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("metadata ref contains non-regular entry %q", h.Name)
		}
		if err := w.WriteHeader(&tar.Header{Name: h.Name, Mode: 0644, Typeflag: tar.TypeReg, Size: h.Size}); err != nil {
			return nil, err
		}
		if _, err := io.CopyN(w, r, h.Size); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return normalized.Bytes(), nil
}

// GetGitStatus returns the git status of the hasura/metadata/ directory.
func GetGitStatus(projectDir string) (HasuraGitStatus, error) {
	branchOut, err := exec.Command("git", "-C", projectDir, "branch", "--show-current").Output()
	if err != nil {
		return HasuraGitStatus{}, fmt.Errorf("git branch --show-current: %w", err)
	}
	branch := strings.TrimSpace(string(branchOut))

	hashOut, err := exec.Command("git", "-C", projectDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return HasuraGitStatus{}, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	commitHash := strings.TrimSpace(string(hashOut))
	if len(commitHash) > 7 {
		commitHash = commitHash[:7]
	}

	statusOut, err := exec.Command("git", "-C", projectDir, "status", "--porcelain", "hasura/metadata/").Output()
	if err != nil {
		return HasuraGitStatus{}, fmt.Errorf("git status hasura/metadata/: %w", err)
	}

	var modified []string
	for _, line := range strings.Split(string(statusOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// porcelain format: XY filename
		if len(line) > 3 {
			modified = append(modified, line[3:])
		}
	}

	return HasuraGitStatus{
		Branch:     branch,
		CommitHash: commitHash,
		Modified:   modified,
		IsClean:    len(modified) == 0,
	}, nil
}

// ApplyMetadataFromGit checks out the metadata files at the given git ref and
// then applies them via the standard metadata apply mechanism.
// ref can be a branch name, tag, or commit hash.
func ApplyMetadataFromGit(ctx context.Context, cfg *config.Config, projectDir string, ref string) error {
	checkoutCmd := exec.CommandContext(ctx, "git", "-C", projectDir, "checkout", ref, "--", "hasura/metadata/")
	if out, err := checkoutCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s -- hasura/metadata/: %w\n%s", ref, err, strings.TrimSpace(string(out)))
	}

	if err := HasuraApplyMetadata(ctx, cfg, projectDir); err != nil {
		return fmt.Errorf("apply metadata from ref %s: %w", ref, err)
	}
	return nil
}

// GenerateTrackTableSQL generates a comment block describing how to track a new
// table in Hasura metadata. It returns a YAML snippet and CLI hint rather than
// raw SQL because table tracking is a Hasura metadata operation.
func GenerateTrackTableSQL(schema, table string) string {
	return fmt.Sprintf(`-- Track table in Hasura
-- Run: nself db hasura metadata apply
-- Or add to hasura/metadata/databases/default/tables/%s_%s.yaml:
--
-- table:
--   schema: %s
--   name: %s
`, schema, table, schema, table)
}

// MetadataSnapshot captures the current live metadata as a JSON snapshot.
// Useful for comparing before/after schema migrations.
func MetadataSnapshot(ctx context.Context, cfg *config.Config) ([]byte, error) {
	data, err := postMetadata(ctx, cfg, metadataRequest{
		Type: "export_metadata",
		Args: map[string]interface{}{},
	})
	if err != nil {
		return nil, fmt.Errorf("capture metadata snapshot: %w", err)
	}
	return data, nil
}

// CompareSnapshots returns a human-readable diff between two metadata snapshots.
// It compares table lists extracted from the sources and reports additions and removals.
func CompareSnapshots(before, after []byte) (string, error) {
	beforeTables, err := extractTablesFromMetadata(before)
	if err != nil {
		return "", fmt.Errorf("parse before snapshot: %w", err)
	}
	afterTables, err := extractTablesFromMetadata(after)
	if err != nil {
		return "", fmt.Errorf("parse after snapshot: %w", err)
	}

	beforeSet := make(map[string]bool, len(beforeTables))
	for _, t := range beforeTables {
		beforeSet[t.Schema+"."+t.Name] = true
	}
	afterSet := make(map[string]bool, len(afterTables))
	for _, t := range afterTables {
		afterSet[t.Schema+"."+t.Name] = true
	}

	var added, removed []string
	for k := range afterSet {
		if !beforeSet[k] {
			added = append(added, k)
		}
	}
	for k := range beforeSet {
		if !afterSet[k] {
			removed = append(removed, k)
		}
	}

	if len(added) == 0 && len(removed) == 0 {
		return "No changes between snapshots.", nil
	}

	var sb strings.Builder
	sb.WriteString("Snapshot diff:\n")
	for _, t := range added {
		fmt.Fprintf(&sb, "  + %s\n", t)
	}
	for _, t := range removed {
		fmt.Fprintf(&sb, "  - %s\n", t)
	}
	return sb.String(), nil
}
