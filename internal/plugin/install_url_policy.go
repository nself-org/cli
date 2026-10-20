package plugin

// URL installs have no signing authority. Carry the explicit opt-in in the
// invocation context so it cannot leak into later installs in the same process.
import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

type unsignedURLKey struct{}

// WithUnsignedURLAllowed records the explicit URL-install opt-in for this invocation.
func WithUnsignedURLAllowed(ctx context.Context, allow bool) context.Context {
	return context.WithValue(ctx, unsignedURLKey{}, allow)
}

// CheckUnsignedURLInstall enforces the v1.5 opt-in before any download.
func CheckUnsignedURLInstall(ctx context.Context) error {
	allow, _ := ctx.Value(unsignedURLKey{}).(bool)
	// compat.V15(D-0842): URL installs need only confirmation -> require --allow-unsigned
	if compat.V15() && !allow {
		return errs.New("E107", "URL plugins are unsigned; pass --allow-unsigned to install from a URL")
	}
	return nil
}

// Only replacements need a registry lookup. Fail closed if the registry cannot
// establish that an existing directory belongs to an unofficial plugin.
func checkURLReplacement(ctx context.Context, pluginDir, name string) error {
	if _, err := os.Lstat(filepath.Join(pluginDir, name)); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	reg, err := FetchRegistry(ctx, os.Getenv("NSELF_PLUGIN_REGISTRY"), DefaultCacheDir())
	if err != nil {
		return fmt.Errorf("checking URL replacement against registry: %w", err)
	}
	if _, found := FindPluginByName(reg, name); found {
		return fmt.Errorf("refusing to replace registry plugin %q from a third-party URL", name)
	}
	return nil
}
