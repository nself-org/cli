package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

func promoteFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `schema_version: 2
project: fixture
environments:
  local:
    name: local
    kind: local
    tier: local
    servers: []
  ls:
    name: ls
    kind: remote
    tier: local-servers
    servers:
      - {name: ls-app, role: app, host: tester@ls.example.invalid, ssh_key_ref: TEST_KEY, remote_path: /srv/fixture, primary: true, arch: amd64}
  prodsim:
    name: prodsim
    kind: remote
    tier: prod
    servers:
      - {name: prod-app, role: app, host: tester@prod.example.invalid, ssh_key_ref: TEST_KEY, remote_path: /srv/fixture, primary: true, arch: amd64}
`
	if err := os.WriteFile(filepath.Join(dir, ".nself", "control-plane.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

func promoteTestCommand(out *bytes.Buffer, dry, yes, jsonOut bool) *cobra.Command {
	c := &cobra.Command{Use: "promote"}
	c.SetOut(out)
	c.Flags().Bool("dry-run", dry, "")
	c.Flags().Bool("yes", yes, "")
	c.Flags().Bool("json", jsonOut, "")
	return c
}

func TestPromoteDryRun(t *testing.T) {
	compattest.Set(t, true)
	golden, err := os.ReadFile(filepath.Join("testdata", "json", "deploy-promote.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	promoteFixture(t)
	var out bytes.Buffer
	cmd := promoteTestCommand(&out, true, false, false)
	if err := runPromoteEnvironments(cmd, "ls", "prodsim"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := preparePromoteEngine(cmd, mustProjectRoot(t), "local", "ls", true, false); err != nil {
		t.Fatalf("local source dry-run: %v", err)
	}
	for _, name := range []string{"resolve", "confirm", "verify", "preflight", "source-health", "capture", "build", "compare", "snapshot", "save", "apply", "migrate", "health", "record"} {
		if !strings.Contains(out.String(), name) {
			t.Errorf("plan missing %s", name)
		}
	}
	if err := runPromoteEnvironments(promoteTestCommand(&bytes.Buffer{}, false, false, false), "ls", "prodsim"); errs.ExitCodeFor(err) != 4 {
		t.Fatalf("prod without --yes: %v (exit %d), want E403/4", err, errs.ExitCodeFor(err))
	}
	var jsonDoc bytes.Buffer
	oldWriter := pilotWriter
	pilotWriter = func() output.Writer { return output.Writer{Out: &jsonDoc, Err: &bytes.Buffer{}} }
	t.Cleanup(func() { pilotWriter = oldWriter; output.ResetState() })
	if err := runPromoteEnvironments(promoteTestCommand(&bytes.Buffer{}, true, false, true), "ls", "prodsim"); err != nil {
		t.Fatal(err)
	}
	if bytes.Count(jsonDoc.Bytes(), []byte(`"schema_version"`)) != 1 || !bytes.Contains(jsonDoc.Bytes(), []byte(`"dry_run": true`)) {
		t.Fatalf("dry-run JSON is not one envelope: %s", jsonDoc.Bytes())
	}
	if !bytes.Equal(jsonDoc.Bytes(), golden) {
		t.Fatalf("dry-run JSON differs from golden\n--- got ---\n%s\n--- want ---\n%s", jsonDoc.Bytes(), golden)
	}
}

func TestPromoteArity(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		promoteFixture(t)
		cmd := promoteTestCommand(&bytes.Buffer{}, true, false, false)
		if err := runDeployPromote(cmd, nil); err != nil {
			t.Fatalf("zero-argument canary path: %v", err)
		}
		err := runDeployPromote(cmd, []string{"only-one"})
		if compat.V15() {
			if errs.ExitCodeFor(err) != 1 || !strings.Contains(err.Error(), "E401") {
				t.Fatalf("v1.5 one arg = %v, want E401", err)
			}
		} else if err != nil && strings.Contains(err.Error(), "E401") {
			t.Fatalf("v1.4 interpreted args: %v", err)
		} else {
			old, _ := promoteCmd.Flags().GetBool("dry-run")
			_ = promoteCmd.Flags().Set("dry-run", "true")
			t.Cleanup(func() { _ = promoteCmd.Flags().Set("dry-run", fmt.Sprint(old)) })
			if legacyErr := promoteCmd.RunE(promoteCmd, []string{"ls", "prodsim"}); legacyErr != nil {
				t.Fatalf("v1.4 promote did not reach internal/promote dry-run: %v", legacyErr)
			}
		}
	})
}

func TestPromoteEquivalence(t *testing.T) {
	compattest.Set(t, true)
	promoteFixture(t)
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	defer undo()
	deployDry, _ := deployPromoteCmd.Flags().GetBool("dry-run")
	promoteDry, _ := promoteCmd.Flags().GetBool("dry-run")
	deployOut, promoteOut := deployPromoteCmd.OutOrStdout(), promoteCmd.OutOrStdout()
	t.Cleanup(func() {
		_ = deployPromoteCmd.Flags().Set("dry-run", fmt.Sprint(deployDry))
		_ = promoteCmd.Flags().Set("dry-run", fmt.Sprint(promoteDry))
		deployPromoteCmd.SetOut(deployOut)
		promoteCmd.SetOut(promoteOut)
	})
	oldArgs := os.Args
	os.Args = []string{"nself", "--quiet"}
	t.Cleanup(func() { os.Args = oldArgs })
	resetCanonWarning()
	var warnings bytes.Buffer
	run := func(argv string) string {
		rewritten, notes, err := rewriteCanonArgsWith(&canonTable, RootCmd, strings.Fields(argv), true)
		if err != nil {
			t.Fatal(err)
		}
		emitCanonWarning(&warnings, strings.Fields(argv), notes, "", "deploy promote")
		cmd, args, err := RootCmd.Find(rewritten)
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set("dry-run", "true"); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := cmd.RunE(cmd, args); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	canonical := run("deploy promote ls prodsim")
	legacy := run("deploy promote-env ls prodsim")
	top := run("promote ls prodsim")
	if canonical != legacy || canonical != top {
		t.Fatalf("promotion spellings produced different plans")
	}
	if got := strings.Count(warnings.String(), "warning:"); got != 1 {
		t.Fatalf("deprecation warnings = %d, want one: %q", got, warnings.String())
	}
}

func mustProjectRoot(t *testing.T) string {
	t.Helper()
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}
