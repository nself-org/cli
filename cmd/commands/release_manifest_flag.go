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
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/controlplane/promote/verify"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin/mount"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const releaseManifestKey = "release_manifest"

// ciPluginBinaryFn finds the installed ci plugin's mounted binary. installed is
// true when a ci plugin is present at all; an entry that cannot be mounted
// (bad binary, collision) reports installed with an empty binary, so a broken
// plugin makes verification required but impossible instead of silently
// optional. A variable so tests can supply a fake plugin.
var ciPluginBinaryFn = func() (binary string, installed bool) {
	specs, problems := mount.Discover(resolvePluginDir(), canonTable.Verbs)
	return selectCIPlugin(specs, problems)
}

// selectCIPlugin picks the ci plugin out of a discovery result. A problem that
// names ci, alone or inside a collision list ("ci,other"), and a ci plugin that
// would not mount as `ci`, count as installed but unusable.
func selectCIPlugin(specs []mount.Spec, problems []mount.Problem) (binary string, installed bool) {
	for _, s := range specs {
		if s.Slug == "ci" && s.Command == "ci" {
			return s.BinaryPath, true
		}
	}
	for _, s := range specs {
		if s.Slug == "ci" {
			return "", true
		}
	}
	for _, p := range problems {
		for _, slug := range strings.Split(p.Slug, ",") {
			if strings.TrimSpace(slug) == "ci" {
				return "", true
			}
		}
	}
	return "", false
}

func init() {
	const usage = "Release directory holding release-manifest.dsse.json and evidence.json; verified before any target change"
	// deploy promote-env (promoteCmd) shares the engine, so it takes the flag
	// too: without it a required target could never be promoted that way. The
	// hidden `nself promote` shim has no flags and fails closed on such targets.
	for _, c := range []*cobra.Command{deployPromoteCmd, promoteCmd} {
		c.Flags().String("release", "", usage)
	}
	// Without NSELF_V15 the legacy promote runs its own v1.4 engine, which
	// never reads a release: accepting the flag there would claim a
	// verification that did not happen.
	promoteCmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		// compat.V15(P7-DEPL-19): v1.4 promote never verifies a release -> --release is refused unless the v1.5 engine runs
		if release, _ := cmd.Flags().GetString("release"); release != "" && !compat.V15() {
			return errs.New("E401", "--release needs the v1.5 promote engine (set NSELF_V15=1): this form does not verify a release")
		}
		return nil
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
	inv, err := controlplane.Load(workdir)
	if err != nil {
		return err
	}
	required := verifiedReleaseRequired(inv.Environments[st.Target], ciInstalled)
	switch {
	case release == "" && required:
		return errs.Newf("E499", "target %s requires a verified release: pass --release <dir>", st.Target)
	case release == "":
		if err := committedGate(cmd.Context(), workdir, st.Target, ciInstalled); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: promoting without --release; the release manifest and its evidence are not verified")
		return nil
	case !ciInstalled:
		return errs.New("E499", "the ci plugin is not installed, so --release cannot be verified")
	case binary == "":
		return errs.New("E499", "the ci plugin is installed but cannot be mounted, so --release cannot be verified")
	case st.DryRun:
		return nil // the plan is printed; nothing is verified or changed
	}
	dir, err := filepath.Abs(release)
	if err != nil {
		return errs.Wrap("E492", "cannot resolve the --release directory", err)
	}
	home, _ := os.UserHomeDir()
	in := verify.Input{
		Dir: dir, RepoRoot: workdir, Home: home,
		Git:      verify.GitCLI{Root: workdir},
		Evidence: verify.ExecEvidence(binary, resolvePluginDir(), workdir),
		Warn:     func(msg string) { _, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning:", msg) },
		// D-1216: trust on first use for the trust origin.
		PinOnFirstUse: true,
		Notice:        func(msg string) { _, _ = fmt.Fprintln(cmd.ErrOrStderr(), msg) },
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

// committedRequiresFn reads whether the inventory committed on origin's default
// branch makes a verified release mandatory for target. available is false when
// there is nothing committed to consult (no origin, or no committed inventory).
// A variable so tests can avoid git.
var committedRequiresFn = committedRequires

func committedRequires(ctx context.Context, workdir, target string, ciInstalled bool) (required, available bool, err error) {
	g := verify.GitCLI{Root: workdir}
	ok, err := g.HasOrigin(ctx)
	if err != nil {
		return false, false, err
	}
	home, _ := os.UserHomeDir()
	// A pinned trust origin applies to committed policy too: a redirected
	// origin must not supply an inventory that waves the gate through.
	if err := verify.CheckOriginPin(ctx, g, home, ok); err != nil {
		return false, false, err
	}
	if !ok {
		return false, false, nil
	}
	commit, err := g.PinDefault(ctx)
	if err != nil {
		return false, false, err
	}
	data, found, err := g.Show(ctx, commit, ".nself/control-plane.yaml")
	if err != nil || !found {
		return false, false, err
	}
	var inv controlplane.Inventory
	if err := yaml.Unmarshal(data, &inv); err != nil {
		return false, false, err
	}
	return verifiedReleaseRequired(inv.Environments[target], ciInstalled), true, nil
}

// committedGate runs when the working-tree inventory did not require a
// verified release. The working tree is not a trust root, so the requirement is
// the stricter of the two copies: an edit of require_verified_release or the
// tier in the working tree cannot switch the gate off. Where the committed copy
// cannot be read the promotion is refused only if the ci plugin is installed,
// because only then could the default requirement have applied. Nothing
// committed to consult (no origin, no committed inventory) is allowed, except
// for an operator who pinned the trust origin (~/.nself/ci/trust/origin.url):
// that operator asked for strictness, so with the ci plugin installed it refuses.
func committedGate(ctx context.Context, workdir, target string, ciInstalled bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	required, available, err := committedRequiresFn(ctx, workdir, target, ciInstalled)
	home, _ := os.UserHomeDir()
	switch {
	case err != nil && ciInstalled:
		return errs.Wrap("E499", "cannot read the inventory committed on origin to confirm no verified release is required", err).WithFix("Check that origin is reachable, or pass --release <dir>.")
	case err != nil:
		return nil
	case required:
		return errs.Newf("E499", "the inventory committed on origin requires a verified release for %s; the working copy says otherwise", target)
	case !available && ciInstalled && verify.OriginPinned(home):
		return errs.Newf("E499", "no inventory is committed on origin to confirm %s needs no verified release, and the trust origin is pinned: pass --release <dir>", target)
	}
	return nil
}
