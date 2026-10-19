package commands

// Purpose: the --release flag of `deploy promote <source> <target>` and the
//          wiring of release verification (P7-DEPL-19, EPIC DEPL D20).
// Inputs:  the flag, the target's inventory require_verified_release field, and
//          whether the ci plugin is installed.
// Outputs: the promote engine's Verify hook and a digest check that runs once
//          promote has compared image identities, both before any snapshot.
// Constraints: no bypass flag or env var. Evidence semantics stay in the ci
//              plugin; this file only calls it. A target that requires a
//              verified release and has none refuses with E499.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/controlplane/promote/verify"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin/mount"
	"github.com/spf13/cobra"
)

const releaseManifestKey = "release_manifest"

// ciPluginBinaryFn finds the installed ci plugin's mounted binary. A variable
// so tests can supply a fake plugin.
var ciPluginBinaryFn = func() (string, bool) {
	specs, _ := mount.Discover(resolvePluginDir(), canonTable.Verbs)
	for _, s := range specs {
		if s.Slug == "ci" && s.Command == "ci" {
			return s.BinaryPath, true
		}
	}
	return "", false
}

func init() {
	const usage = "Release directory holding release-manifest.dsse.json and evidence.json; verified before any target change"
	// The deprecated promote and promote-env forms share the engine, so they
	// take the flag too: without it a required target could never be promoted.
	for _, c := range []*cobra.Command{deployPromoteCmd, promoteCmd} {
		c.Flags().String("release", "", usage)
	}
}

// verifiedReleaseRequired applies the inventory field; the default is true for
// tier prod when the ci plugin is installed.
func verifiedReleaseRequired(env controlplane.Environment, ciInstalled bool) bool {
	if env.RequireVerifiedRelease != nil {
		return *env.RequireVerifiedRelease
	}
	return env.Tier == controlplane.TierProd && ciInstalled
}

// wirePromoteRelease installs release verification on eng, or refuses.
func wirePromoteRelease(cmd *cobra.Command, workdir string, eng *cpromote.Engine, st *cpromote.State) error {
	release, _ := cmd.Flags().GetString("release")
	binary, ciInstalled := ciPluginBinaryFn()
	var env controlplane.Environment
	if inv, err := controlplane.Load(workdir); err == nil {
		env = inv.Environments[st.Target]
	}
	required := verifiedReleaseRequired(env, ciInstalled)
	switch {
	case release == "" && required:
		return errs.Newf("E499", "target %s requires a verified release: pass --release <dir>", st.Target)
	case release == "":
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: promoting without --release; the release manifest and its evidence are not verified")
		return nil
	case st.DryRun:
		return nil
	case !ciInstalled:
		return errs.New("E499", "the ci plugin is not installed, so --release cannot be verified")
	}
	dir, err := filepath.Abs(release)
	if err != nil {
		return errs.Wrap("E492", "cannot resolve the --release directory", err)
	}
	home, _ := os.UserHomeDir()
	in := verify.Input{
		Dir: dir, RepoRoot: workdir, Home: home,
		Git:      verify.GitCLI{Root: workdir},
		Evidence: verify.ExecEvidence(binary, workdir),
		Warn:     func(msg string) { _, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning:", msg) },
	}
	eng.Verify = func(ctx context.Context, s *cpromote.State) error {
		m, err := verify.Verify(ctx, in)
		if err == nil {
			s.Values[releaseManifestKey] = m
		}
		return err
	}
	wrapCompareWithDigestCheck(eng)
	return nil
}

// wrapCompareWithDigestCheck makes the compare step also prove that every
// per-platform digest promote will deploy is listed in the verified manifest.
// Compare runs before the snapshot, so a refusal changes nothing.
func wrapCompareWithDigestCheck(eng *cpromote.Engine) {
	for i := range eng.Steps {
		if eng.Steps[i].Name != "compare" {
			continue
		}
		inner := eng.Steps[i].Run
		eng.Steps[i].Run = func(ctx context.Context, st *cpromote.State) error {
			if inner != nil {
				if err := inner(ctx, st); err != nil {
					return err
				}
			}
			m, _ := st.Values[releaseManifestKey].(*verify.Manifest)
			return verify.CheckDigests(m, st.Digests)
		}
	}
}
