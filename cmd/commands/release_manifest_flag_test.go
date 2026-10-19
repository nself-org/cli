package commands

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/controlplane/promote/verify"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

// The suite must not depend on a ci plugin installed on the machine running it.
func init() { ciPluginBinaryFn = func() (string, bool) { return "", false } }

func withCIPlugin(t *testing.T, installed bool) { withCIBinary(t, "/nonexistent/nself-ci", installed) }

func withCIBinary(t *testing.T, binary string, installed bool) {
	t.Helper()
	old := ciPluginBinaryFn
	ciPluginBinaryFn = func() (string, bool) { return binary, installed }
	t.Cleanup(func() { ciPluginBinaryFn = old })
}

func releaseCmd(t *testing.T, release string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := &cobra.Command{Use: "promote"}
	cmd.Flags().String("release", release, "")
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	return cmd, &stderr
}

func promoteStateFor(inv string, tier controlplane.Tier, dry bool) *cpromote.State {
	return &cpromote.State{Source: "ls", Target: inv, TargetTier: tier, DryRun: dry, Values: map[string]any{}}
}

func codeIs(err error, code string) bool {
	var c *errs.CLIError
	return errors.As(err, &c) && c.Code == code
}

// TestReleaseVerifyRequired covers require_verified_release: the default,
// the inventory override, and the warning for unverified promotions.
func TestReleaseVerifyRequired(t *testing.T) {
	dir := promoteFixture(t)
	t.Run("prod with ci plugin and no release refuses", func(t *testing.T) {
		withCIPlugin(t, true)
		cmd, _ := releaseCmd(t, "")
		err := wirePromoteRelease(cmd, dir, &cpromote.Engine{}, promoteStateFor("prodsim", controlplane.TierProd, false))
		if !codeIs(err, "E499") {
			t.Fatalf("err = %v, want E499", err)
		}
	})
	t.Run("dry run also refuses early", func(t *testing.T) {
		withCIPlugin(t, true)
		cmd, _ := releaseCmd(t, "")
		err := wirePromoteRelease(cmd, dir, &cpromote.Engine{}, promoteStateFor("prodsim", controlplane.TierProd, true))
		if !codeIs(err, "E499") {
			t.Fatalf("err = %v, want E499", err)
		}
	})
	t.Run("prod without ci plugin warns and proceeds", func(t *testing.T) {
		withCIPlugin(t, false)
		cmd, stderr := releaseCmd(t, "")
		eng := &cpromote.Engine{}
		if err := wirePromoteRelease(cmd, dir, eng, promoteStateFor("prodsim", controlplane.TierProd, false)); err != nil || eng.Verify != nil {
			t.Fatalf("err = %v verify set = %v", err, eng.Verify != nil)
		}
		if strings.Count(stderr.String(), "without --release") != 1 {
			t.Fatalf("stderr = %q, want one warning", stderr.String())
		}
	})
	t.Run("local-servers target warns once and proceeds", func(t *testing.T) {
		withCIPlugin(t, true)
		cmd, stderr := releaseCmd(t, "")
		if err := wirePromoteRelease(cmd, dir, &cpromote.Engine{}, promoteStateFor("ls", controlplane.TierLocalServers, false)); err != nil {
			t.Fatal(err)
		}
		if strings.Count(stderr.String(), "without --release") != 1 {
			t.Fatalf("stderr = %q, want one warning", stderr.String())
		}
	})
	t.Run("release given but ci plugin missing refuses", func(t *testing.T) {
		withCIPlugin(t, false)
		cmd, _ := releaseCmd(t, t.TempDir())
		err := wirePromoteRelease(cmd, dir, &cpromote.Engine{}, promoteStateFor("prodsim", controlplane.TierProd, false))
		if !codeIs(err, "E499") {
			t.Fatalf("err = %v, want E499", err)
		}
	})
	t.Run("installed but unmountable ci plugin is required and refuses", func(t *testing.T) {
		withCIBinary(t, "", true)
		cmd, _ := releaseCmd(t, "")
		if err := wirePromoteRelease(cmd, dir, &cpromote.Engine{}, promoteStateFor("prodsim", controlplane.TierProd, false)); !codeIs(err, "E499") {
			t.Fatalf("no release: %v, want E499 (a broken plugin must not make verification optional)", err)
		}
		cmd, _ = releaseCmd(t, t.TempDir())
		err := wirePromoteRelease(cmd, dir, &cpromote.Engine{}, promoteStateFor("prodsim", controlplane.TierProd, false))
		if !codeIs(err, "E499") || !strings.Contains(err.Error(), "cannot be mounted") {
			t.Fatalf("with release: %v, want E499 cannot be mounted", err)
		}
	})
	t.Run("release given installs the hook and the digest check", func(t *testing.T) {
		withCIPlugin(t, true)
		cmd, _ := releaseCmd(t, t.TempDir())
		compared := false
		eng := &cpromote.Engine{Steps: []cpromote.Step{{Name: "verify"}, {Name: "compare", Run: func(context.Context, *cpromote.State) error { compared = true; return nil }}}}
		st := promoteStateFor("prodsim", controlplane.TierProd, false)
		if err := wirePromoteRelease(cmd, dir, eng, st); err != nil || eng.Verify == nil {
			t.Fatalf("err = %v verify set = %v", err, eng.Verify != nil)
		}
		// Without a verified manifest in state the wrapped compare refuses (E499).
		if err := eng.Steps[1].Run(context.Background(), st); !compared || !codeIs(err, "E499") {
			t.Fatalf("compare ran = %v err = %v, want E499 after compare", compared, err)
		}
		st.Values[releaseManifestKey] = &verify.Manifest{}
		st.Digests = []cpromote.DigestRow{{Service: "web", Target: "sha256:" + strings.Repeat("9", 64)}}
		if err := eng.Steps[1].Run(context.Background(), st); !codeIs(err, "E495") {
			t.Fatalf("err = %v, want E495", err)
		}
	})
}

func TestVerifiedReleaseRequiredDefaults(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name string
		env  controlplane.Environment
		ci   bool
		want bool
	}{
		{"prod default with ci", controlplane.Environment{Tier: controlplane.TierProd}, true, true},
		{"prod default without ci", controlplane.Environment{Tier: controlplane.TierProd}, false, false},
		{"local-servers default", controlplane.Environment{Tier: controlplane.TierLocalServers}, true, false},
		{"explicit true on local-servers", controlplane.Environment{Tier: controlplane.TierLocalServers, RequireVerifiedRelease: &yes}, false, true},
		{"explicit false on prod", controlplane.Environment{Tier: controlplane.TierProd, RequireVerifiedRelease: &no}, true, false},
	}
	for _, tc := range cases {
		if got := verifiedReleaseRequired(tc.env, tc.ci); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestReleaseFlagRefusedOnTheCanaryForm: --release must not be silently
// ignored by the zero-argument canary flip.
func TestReleaseFlagRefusedOnTheCanaryForm(t *testing.T) {
	cmd, _ := releaseCmd(t, t.TempDir())
	if err := runDeployPromoteBody(cmd, nil); !codeIs(err, "E401") {
		t.Fatalf("err = %v, want E401", err)
	}
}
