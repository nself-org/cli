package build

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

func TestBuildEntitlementFailureCompletesArtifacts(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	dir, fragment := entitlementFixture(t, "licensed")
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	writeFixtureFile(t, filepath.Join(dir, "fixture", pluginComposeFilename), string(fragment), 0644)
	writeFixtureFile(t, filepath.Join(dir, "free", "plugin.json"), `{"name":"free","license":"free"}`, 0644)
	writeFixtureFile(t, filepath.Join(dir, "free", pluginComposeFilename), "services:\n  free:\n    image: fake/free\n", 0644)
	old := refreshPluginEntitlement
	t.Cleanup(func() { refreshPluginEntitlement = old })
	failure := errs.New("E125", "no entitlement token")
	refreshPluginEntitlement = func(context.Context, string, string) error { return failure }
	result, err := Build(f.workdir, BuildOptions{Force: true})
	if !errors.Is(err, failure) || result == nil {
		t.Fatalf("want completed result and entitlement failure, got %+v, %v", result, err)
	}
	for _, name := range []string{composeManifestFile, ".env.computed", composeEnvFile, buildVersionFile} {
		path := filepath.Join(f.workdir, name)
		data, readErr := os.ReadFile(path)
		if readErr != nil || len(data) == 0 {
			t.Errorf("later artifact %s missing: %v", name, readErr)
		}
	}
	manifest, _ := os.ReadFile(filepath.Join(f.workdir, composeManifestFile))
	for _, plugin := range []string{"fixture", "free"} {
		if !strings.Contains(string(manifest), filepath.Join(f.workdir, ".nself", "plugins", plugin, pluginComposeFilename)) {
			t.Fatalf("%s fragment absent from completed manifest", plugin)
		}
	}
	if !result.EntitlementRefreshed {
		t.Fatal("start would validate twice after auto-build")
	}
}

func TestBuildEntitlementFailureDoesNotHideLaterError(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	dir, fragment := entitlementFixture(t, "licensed")
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	writeFixtureFile(t, filepath.Join(dir, "fixture", pluginComposeFilename), string(fragment), 0644)
	if err := os.Mkdir(filepath.Join(f.workdir, ".env.computed"), 0755); err != nil {
		t.Fatal(err)
	}
	old := refreshPluginEntitlement
	t.Cleanup(func() { refreshPluginEntitlement = old })
	refreshPluginEntitlement = func(context.Context, string, string) error { return errs.New("E125", "no token") }
	result, err := Build(f.workdir, BuildOptions{Force: true})
	if result != nil || err == nil || !strings.Contains(err.Error(), "writing .env.computed") || StartMayContinue(err, &bytes.Buffer{}) {
		t.Fatalf("later build error must stay fatal, got %+v, %v", result, err)
	}
}

func TestStartBuildErrorPolicy(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"success", nil, true},
		{"entitlement", wrapEntitlementRefresh(errs.New("E125", "no token")), true},
		{"fragment", errs.New("E125", "invalid licensed fragment"), false},
		{"other coded refresh", wrapEntitlementRefresh(errs.New("E126", "other failure")), false},
		{"other build", errs.New("E128", "missing fragment"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := errs.ExitCodeFor(tc.err)
			var warnings bytes.Buffer
			if got := StartMayContinue(tc.err, &warnings); got != tc.want || errs.ExitCodeFor(tc.err) != code {
				t.Fatalf("continuation=%v want=%v; error code changed or wrong policy", got, tc.want)
			}
		})
	}
}

func TestBuildLicensedFragmentErrorStaysFatal(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	dir, _ := entitlementFixture(t, "licensed")
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	writeFixtureFile(t, filepath.Join(dir, "fixture", pluginComposeFilename), "services: !reset []\n", 0644)
	result, err := Build(f.workdir, BuildOptions{Force: true})
	if result != nil || errs.Describe(err) == nil || errs.Describe(err).Code != "E125" || errs.ExitCodeFor(err) != 3 || StartMayContinue(err, &bytes.Buffer{}) {
		t.Fatalf("invalid licensed fragment must stay E125 exit 3, got %+v, %v", result, err)
	}
}

func TestBuildOtherRefreshErrorStaysFatal(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	dir, fragment := entitlementFixture(t, "licensed")
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	writeFixtureFile(t, filepath.Join(dir, "fixture", pluginComposeFilename), string(fragment), 0644)
	old := refreshPluginEntitlement
	t.Cleanup(func() { refreshPluginEntitlement = old })
	failure := errs.New("E126", "different refresh failure")
	refreshPluginEntitlement = func(context.Context, string, string) error { return failure }
	result, err := Build(f.workdir, BuildOptions{Force: true})
	if result != nil || !errors.Is(err, failure) || errs.ExitCodeFor(err) != errs.ExitCodeFor(failure) || StartMayContinue(err, &bytes.Buffer{}) {
		t.Fatalf("other refresh error must stay fatal with its code, got %+v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(f.workdir, composeManifestFile)); !os.IsNotExist(err) {
		t.Fatalf("non-entitlement failure continued into manifest publication: %v", err)
	}
}
