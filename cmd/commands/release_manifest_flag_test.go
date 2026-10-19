package commands

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/controlplane/promote/verify"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin/mount"
	"github.com/spf13/cobra"
)

// The suite must not depend on a ci plugin installed on the machine running it,
// nor on git or the network.
func init() {
	ciPluginBinaryFn = func() (string, bool) { return "", false }
	committedRequiresFn = func(context.Context, string, string, bool) (bool, error) { return false, nil }
}

func withCommitted(t *testing.T, required bool, err error) {
	t.Helper()
	old := committedRequiresFn
	committedRequiresFn = func(context.Context, string, string, bool) (bool, error) { return required, err }
	t.Cleanup(func() { committedRequiresFn = old })
}

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
	t.Run("dry run with a release still needs a usable ci plugin", func(t *testing.T) {
		withCIPlugin(t, false)
		cmd, _ := releaseCmd(t, t.TempDir())
		if err := wirePromoteRelease(cmd, dir, &cpromote.Engine{}, promoteStateFor("prodsim", controlplane.TierProd, true)); !codeIs(err, "E499") {
			t.Fatalf("missing plugin: %v, want E499", err)
		}
		withCIBinary(t, "", true)
		if err := wirePromoteRelease(cmd, dir, &cpromote.Engine{}, promoteStateFor("prodsim", controlplane.TierProd, true)); !codeIs(err, "E499") {
			t.Fatalf("unmountable plugin: %v, want E499", err)
		}
		withCIPlugin(t, true)
		eng := &cpromote.Engine{}
		if err := wirePromoteRelease(cmd, dir, eng, promoteStateFor("prodsim", controlplane.TierProd, true)); err != nil || eng.Verify != nil {
			t.Fatalf("usable plugin: err %v, verify set %v; a dry run verifies nothing", err, eng.Verify != nil)
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

func TestSelectCIPlugin(t *testing.T) {
	ok := mount.Spec{Slug: "ci", Command: "ci", BinaryPath: "/p/nself-ci"}
	cases := []struct {
		name     string
		specs    []mount.Spec
		problems []mount.Problem
		binary   string
		inst     bool
	}{
		{"healthy", []mount.Spec{ok}, nil, "/p/nself-ci", true},
		{"no plugin", nil, nil, "", false},
		{"other plugin only", []mount.Spec{{Slug: "x", Command: "x", BinaryPath: "/p/x"}}, []mount.Problem{{Slug: "y"}}, "", false},
		{"bad manifest", nil, []mount.Problem{{Slug: "ci", Code: "E113"}}, "", true},
		{"verb collision names both", nil, []mount.Problem{{Slug: "ci,other", Code: "E405"}}, "", true},
		{"collision with spaces", nil, []mount.Problem{{Slug: "other, ci"}}, "", true},
		{"similar slug is not ci", nil, []mount.Problem{{Slug: "ci-extra,cix"}}, "", false},
		{"ci mounted under another command", []mount.Spec{{Slug: "ci", Command: "pipeline", BinaryPath: "/p/nself-ci"}}, nil, "", true},
		{"healthy beats a stale problem", []mount.Spec{ok}, []mount.Problem{{Slug: "ci"}}, "/p/nself-ci", true},
	}
	for _, tc := range cases {
		binary, inst := selectCIPlugin(tc.specs, tc.problems)
		if binary != tc.binary || inst != tc.inst {
			t.Errorf("%s: got %q, %v; want %q, %v", tc.name, binary, inst, tc.binary, tc.inst)
		}
	}
}

// TestCommittedInventoryCannotBeWeakenedByTheWorkingTree: the stricter of the
// working copy and origin's committed inventory decides.
func TestCommittedInventoryCannotBeWeakenedByTheWorkingTree(t *testing.T) {
	dir := promoteFixture(t)
	t.Run("committed requires, working tree does not", func(t *testing.T) {
		withCIPlugin(t, true)
		withCommitted(t, true, nil)
		cmd, _ := releaseCmd(t, "")
		// ls is local-servers: the working tree does not require a release.
		err := wirePromoteRelease(cmd, dir, &cpromote.Engine{}, promoteStateFor("ls", controlplane.TierLocalServers, false))
		if !codeIs(err, "E499") || !strings.Contains(err.Error(), "committed on origin") {
			t.Fatalf("err = %v, want E499 naming the committed inventory", err)
		}
	})
	t.Run("committed copy unreadable with the ci plugin installed refuses", func(t *testing.T) {
		withCIPlugin(t, true)
		withCommitted(t, false, errors.New("origin unreachable"))
		cmd, _ := releaseCmd(t, "")
		if err := wirePromoteRelease(cmd, dir, &cpromote.Engine{}, promoteStateFor("ls", controlplane.TierLocalServers, false)); !codeIs(err, "E499") {
			t.Fatalf("err = %v, want E499", err)
		}
	})
	t.Run("committed copy unreadable without the ci plugin warns and proceeds", func(t *testing.T) {
		withCIPlugin(t, false)
		withCommitted(t, false, errors.New("origin unreachable"))
		cmd, stderr := releaseCmd(t, "")
		if err := wirePromoteRelease(cmd, dir, &cpromote.Engine{}, promoteStateFor("ls", controlplane.TierLocalServers, false)); err != nil {
			t.Fatal(err)
		}
		if strings.Count(stderr.String(), "without --release") != 1 {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})
	t.Run("committed does not require", func(t *testing.T) {
		withCIPlugin(t, true)
		withCommitted(t, false, nil)
		cmd, _ := releaseCmd(t, "")
		if err := wirePromoteRelease(cmd, dir, &cpromote.Engine{}, promoteStateFor("ls", controlplane.TierLocalServers, false)); err != nil {
			t.Fatal(err)
		}
	})
}

// TestCommittedRequiresReadsOriginsInventory exercises the real git path.
func TestCommittedRequiresReadsOriginsInventory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a local git remote")
	}
	root := t.TempDir()
	origin, work := filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	git := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "init.defaultBranch=main", "-C", dir}, args...)...)
		c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(root, "init", "--bare", origin)
	git(root, "init", work)
	if err := os.MkdirAll(filepath.Join(work, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	yamlText := "schema_version: 2\nproject: p\nenvironments:\n  prodsim:\n    name: prodsim\n    kind: remote\n    tier: prod\n    require_verified_release: true\n    servers: []\n  ls:\n    name: ls\n    kind: remote\n    tier: local-servers\n    servers: []\n"
	if err := os.WriteFile(filepath.Join(work, ".nself", "control-plane.yaml"), []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}
	git(work, "add", ".")
	git(work, "commit", "-m", "inventory")
	git(work, "remote", "add", "origin", origin)
	git(work, "push", "origin", "HEAD:refs/heads/main")
	real := committedRequires
	if got, err := real(context.Background(), work, "prodsim", false); err != nil || !got {
		t.Fatalf("prodsim: %v, %v; want required", got, err)
	}
	if got, err := real(context.Background(), work, "ls", false); err != nil || got {
		t.Fatalf("ls without ci: %v, %v; want not required", got, err)
	}
	if got, err := real(context.Background(), work, "absent", true); err != nil || got {
		t.Fatalf("unknown env: %v, %v; want not required", got, err)
	}
	// A working-tree edit does not change the committed answer.
	if err := os.WriteFile(filepath.Join(work, ".nself", "control-plane.yaml"), []byte("schema_version: 2\nproject: p\nenvironments: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := real(context.Background(), work, "prodsim", false); err != nil || !got {
		t.Fatalf("after a working-tree edit: %v, %v; want still required", got, err)
	}
	// No origin: nothing committed to consult, not an error.
	git(work, "remote", "remove", "origin")
	if got, err := real(context.Background(), work, "prodsim", true); err != nil || got {
		t.Fatalf("no origin: %v, %v; want false, nil", got, err)
	}
}

func TestReleaseFlagRefusedOnLegacyPromote(t *testing.T) {
	t.Setenv("NSELF_V15", "")
	cmd, _ := releaseCmd(t, t.TempDir())
	if err := promoteCmd.PreRunE(cmd, nil); !codeIs(err, "E401") {
		t.Fatalf("v1.4: %v, want E401", err)
	}
	cmd, _ = releaseCmd(t, "")
	if err := promoteCmd.PreRunE(cmd, nil); err != nil {
		t.Fatalf("no flag: %v", err)
	}
	t.Setenv("NSELF_V15", "1")
	cmd, _ = releaseCmd(t, t.TempDir())
	if err := promoteCmd.PreRunE(cmd, nil); err != nil {
		t.Fatalf("v1.5: %v", err)
	}
}
