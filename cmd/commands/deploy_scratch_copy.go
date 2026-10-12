package commands

// Purpose: the scratch copy of the operator's project for a v1.5 remote deploy
//          (moved out of deploy_scratch.go, which documents the rules).
// Inputs:  the project dir and the scratch dir.
// Outputs: the scratch dir filled with the copy.
// Constraints: see deploy_scratch.go. Marker-carrying nginx/ files are left out
//              (deploy_scratch_nginx.go: scratchLeavesGenerated).

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/deploy"
)

// copyProject copies src to dst without the entries scratchSkipTop and
// scratchSkipAny name and without deploy env snapshots.
func scratchCopyProject(src, dst string, policy *deploy.NginxPolicy) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		top := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		skip := scratchSkipTop[top] || scratchSkipAny[info.Name()] || strings.HasPrefix(info.Name(), ".nself-deploy-env-")
		if skip {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		out := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(out, 0o755)
		case info.Mode()&os.ModeSymlink != 0:
			return scratchCopyLink(src, p, out)
		case info.Mode().IsRegular():
			if scratchLeavesGenerated(p, rel, policy) {
				return nil // stale generated nginx output: the build writes what it owns
			}
			return scratchCopyFile(p, out, info)
		}
		return nil
	})
}

// copyLink recreates a symlink only when it is relative and stays inside src.
func scratchCopyLink(src, link, out string) error {
	target, err := os.Readlink(link)
	if err != nil || filepath.IsAbs(target) {
		return nil
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(link), target))
	if resolved != src && !strings.HasPrefix(resolved, src+string(filepath.Separator)) {
		return nil
	}
	return os.Symlink(target, out)
}

// scratchCopyFile copies one regular file, keeping its mode and mtime; the
// target must not exist yet (O_EXCL), so a copy never writes through a link.
func scratchCopyFile(from, to string, info os.FileInfo) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(to, info.ModTime(), info.ModTime())
}
