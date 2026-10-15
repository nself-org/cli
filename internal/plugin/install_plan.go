package plugin

// Purpose: stage verified registry packages without installing them.
// Inputs: caller-owned temporary root, config and plugin names.
// Outputs: staged files and temporary registry/license caches.
// Constraints: reuse installer policy; never publish binaries or change schemas.

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
)

type planInstallKey struct{}

func planningInstall(ctx context.Context) bool {
	return ctx.Value(planInstallKey{}) != nil
}

func installCacheDir(ctx context.Context, normal, kind string) string {
	if root, ok := ctx.Value(planInstallKey{}).(string); ok {
		return filepath.Join(root, kind)
	}
	return normal
}

// CopyPlanTree copies a plugin/cache tree, refusing links that could escape staging.
func CopyPlanTree(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return os.MkdirAll(dst, 0o700)
	}
	return fs.WalkDir(os.DirFS(src), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "bin" && d.IsDir() {
			return fs.SkipDir
		}
		if path == ".install.lock" {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("cannot stage symlink %s", filepath.Join(src, path))
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, path), 0o700)
		}
		body, err := os.ReadFile(filepath.Join(src, path))
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, path), body, info.Mode().Perm())
	})
}

// StageForPlan runs the ordinary install checks and dependency recursion, stopping
// after verified extraction. Every cache write is confined to root.
func StageForPlan(ctx context.Context, cfg *config.Config, names []string, dir, root string, allowEOL bool) error {
	if err := CopyPlanTree(licenseCacheDir(), filepath.Join(root, "license")); err != nil {
		return err
	}
	ctx = context.WithValue(ctx, planInstallKey{}, root)
	for _, name := range names {
		if err := CheckEOLBlock(ctx, name, allowEOL); err != nil {
			return err
		}
		if err := installLocked(ctx, cfg, name, dir); err != nil {
			return err
		}
	}
	return nil
}

// StageRemoveForPlan applies the ordinary reverse-dependency guard, then removes
// only the caller's staged copy. It never stops containers or drops data.
func StageRemoveForPlan(dir, name string, force bool) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("plugin %q is not installed: %w", name, err)
	}
	if !force {
		deps, err := checkReverseDependencies(dir, name)
		if err != nil {
			return err
		}
		if len(deps) > 0 {
			return fmt.Errorf("plugins %v depend on %q; use --force to remove anyway", deps, name)
		}
	}
	return os.RemoveAll(filepath.Join(dir, name))
}
