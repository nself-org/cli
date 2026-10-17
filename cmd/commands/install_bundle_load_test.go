package commands

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle"
)

// TestInstallLoadsBundleCatalog pins D-0268: a fresh process has an empty bundle
// catalog, so `nself install <bundle>` must load it before resolving names and
// reach the bundle installer instead of the plugin installer.
func TestInstallLoadsBundleCatalog(t *testing.T) {
	t.Setenv("NSELF_BUNDLES_CACHE_PATH", filepath.Join(t.TempDir(), "bundles.json"))
	if err := installCmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resetFlag(installCmd, "dry-run") })
	oldInstall := bundleInstall
	var slugs []string
	var opts []bundle.InstallOpts
	bundleInstall = func(_ context.Context, slug string, o bundle.InstallOpts) (*bundle.InstallResult, error) {
		slugs = append(slugs, slug)
		opts = append(opts, o)
		return &bundle.InstallResult{Bundle: slug, DryRun: o.DryRun}, nil
	}
	t.Cleanup(func() {
		bundleInstall = oldInstall
		_ = bundle.LoadBytes([]byte(fixtureBundlesJSON))
	})
	// A catalog without "chat", as in a fresh process: it is unknown until loaded.
	if err := bundle.LoadBytes([]byte(strings.Replace(fixtureBundlesJSON, `"chat":`, `"other":`, 1))); err != nil {
		t.Fatal(err)
	}
	if resolvesToBundle("chat") {
		t.Fatal("precondition: chat must be unknown before the catalog loads")
	}
	if err := runInstall(installCmd, []string{"chat"}); err != nil {
		t.Fatalf("install chat: %v", err)
	}
	if !reflect.DeepEqual(slugs, []string{"chat"}) || len(opts) != 1 || !opts[0].DryRun {
		t.Fatalf("bundle installer calls = %v %+v, want one dry-run call for chat", slugs, opts)
	}
}
