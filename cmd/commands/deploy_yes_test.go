package commands

// P7-WIRE-13 defect 1: the scratch build deploy and promote spawn inherits the
// caller's --yes, so a staging or prod build does not stop at the confirmation
// (E403) when nobody is at a terminal. Driven by argv through RootCmd.

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// yesBuildRecorder replaces the nself subprocess: it records each build argv and,
// like the real staging/prod build off a terminal, refuses with E403 unless
// --yes is present. It writes the generated compose so the freshness check passes.
func yesBuildRecorder(t *testing.T, got *[][]string) {
	t.Helper()
	old := deployCLISelfFn
	t.Cleanup(func() { deployCLISelfFn = old })
	deployCLISelfFn = func(_ context.Context, workdir string, args ...string) error {
		*got = append(*got, slices.Clone(args))
		if !slices.Contains(args, "--yes") {
			return errs.New("E403", "a prod-class build needs --yes or a terminal")
		}
		return os.WriteFile(filepath.Join(workdir, "docker-compose.yml"), []byte(build.ComposeGeneratedHeader+"\nservices: {}\n"), 0o644)
	}
}

func resetYesFlags() {
	for _, c := range []*cobra.Command{deployCmd, deployPromoteCmd} {
		c.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
	}
	RootCmd.SetArgs(nil)
}

func yesArgv(base []string, yes bool) (string, []string) {
	if yes {
		return "with --yes", append(slices.Clone(base), "--yes")
	}
	return "without --yes", base
}

func TestDeployYesBuildStep(t *testing.T) {
	for _, tc := range []struct {
		name    string
		yes     bool
		wantErr bool
	}{{"with --yes", true, false}, {"without --yes", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			var argv [][]string
			yesBuildRecorder(t, &argv)
			dir := t.TempDir()
			ctx := context.WithValue(context.Background(), deployYesKey{}, tc.yes)
			_, err := runDeployBuildStep(ctx, dir, true, nil)
			if (err != nil) != tc.wantErr || (tc.wantErr && !strings.Contains(err.Error(), "E403")) {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if len(argv) != 1 || slices.Contains(argv[0], "--yes") != tc.yes {
				t.Fatalf("build argv %v", argv)
			}
			if tc.yes && !slices.Equal(argv[0], append(deployBuildArgsForProfile(dir, true, ""), "--yes")) {
				t.Fatalf("--yes is the only new arg: %v", argv[0])
			}
		})
	}
}

// TestDeployYesRootDeploy: `nself deploy qa [--yes]` through the cobra root
// reaches the build step with the caller's --yes.
func TestDeployYesRootDeploy(t *testing.T) {
	for _, yes := range []bool{true, false} {
		name, argv := yesArgv([]string{"deploy", "qa"}, yes)
		t.Run(name, func(t *testing.T) {
			var builds [][]string
			_, _, bin, _ := pushFixture(t)
			pinFakeHostKeys(t, bin, wire21Host)
			t.Setenv("NSELF_DEPLOY_HOST_QA", wire21Host)
			yesBuildRecorder(t, &builds)
			ob := deployBuildStepFn
			deployBuildStepFn = runDeployBuildStep
			t.Cleanup(func() { deployBuildStepFn = ob; resetYesFlags() })
			resetYesFlags()
			RootCmd.SetArgs(argv)
			var err error
			captureStd(t, func() { err = RootCmd.Execute() })
			if len(builds) != 1 || slices.Contains(builds[0], "--yes") != yes {
				t.Fatalf("builds %v err=%v", builds, err)
			}
			if yes && err != nil && strings.Contains(err.Error(), "E403") {
				t.Fatalf("--yes still failed E403: %v", err)
			}
			if !yes && (err == nil || !strings.Contains(err.Error(), "E403")) {
				t.Fatalf("without --yes the build keeps its confirmation: %v", err)
			}
		})
	}
}

// TestDeployYesPromoteBuild: `nself deploy promote ls prodsim [--yes]` runs the
// promote build step's scratch build with the caller's --yes.
func TestDeployYesPromoteBuild(t *testing.T) {
	skipPromoteShellDoubles(t)
	for _, yes := range []bool{true, false} {
		name, argv := yesArgv([]string{"deploy", "promote", "ls", "prodsim"}, yes)
		t.Run(name, func(t *testing.T) {
			compattest.Set(t, true)
			locked := compose.LockedImages()[0]
			setupPromoteSafety(t, locked.LegacyRef, locked.String(), false)
			var builds [][]string
			yesBuildRecorder(t, &builds)
			deployScratchBuildFn = runDeployBuildStep // setupPromoteSafety restores the original on cleanup
			t.Cleanup(resetYesFlags)
			resetYesFlags()
			RootCmd.SetArgs(argv)
			var err error
			captureStd(t, func() { err = RootCmd.Execute() })
			if !yes { // promote's own confirmation is unchanged: it stops before any build
				if len(builds) != 0 || err == nil || !strings.Contains(err.Error(), "E403") {
					t.Fatalf("builds %v err=%v", builds, err)
				}
				return
			}
			if len(builds) != 1 || !slices.Contains(builds[0], "--yes") {
				t.Fatalf("builds %v err=%v", builds, err)
			}
			if err != nil && strings.Contains(err.Error(), "E403") {
				t.Fatalf("--yes still failed E403: %v", err)
			}
		})
	}
}
