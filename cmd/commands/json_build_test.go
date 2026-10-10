package commands

// Tests for the build fragment's envelopes and confirmation rows (P7-SURF-26).
//
// Purpose: prove reset, clean and uninstall answer `--json` with the v1
// envelope in v1.5 mode, print nothing extra in v1.4 mode, refuse (rather than
// report success) when the prompt is declined in JSON mode, and that the
// registry carries the confirm blocks of build, reset, clean and uninstall.
// Inputs: fixture projects under t.TempDir, a docker stub on PATH, real run
// functions behind fresh cobra commands carrying the production flags.
// Outputs: pass/fail. Constraints: no Docker daemon, nothing outside t.TempDir.

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func TestEnvelopeCoverageBuild(t *testing.T) {
	assertFragmentEnvelopeCoverage(t, "build")
}

// buildFixture makes a project with every artifact the three commands remove.
func buildFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		".env":                    "PROJECT_NAME=smoke\nENV=dev\nBASE_DOMAIN=example.test\n",
		"docker-compose.yml":      "services: {}\n",
		".env.computed":           "A=1\n",
		"nginx/sites/app.conf":    "server {}\n",
		".nself/cache/build.json": "{}\n",
		".nself/volumes/pg/data":  "x\n",
	}
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stub := t.TempDir()
	if err := os.WriteFile(filepath.Join(stub, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// runBuildCmd runs the real RunE of name with args (flags like --yes) and
// stdin, returning the pilot stdout and the error.
func runBuildCmd(t *testing.T, name string, stdin string, flags ...string) (string, error) {
	t.Helper()
	out, _ := pilotBuffers(t)
	c := &cobra.Command{Use: name}
	switch name {
	case "clean":
		c.RunE = runClean
		c.Flags().Bool("all", false, "")
	case "reset":
		c.RunE = runReset
		c.Flags().Bool("keep-data", false, "")
		c.Flags().Bool("purge", false, "")
		c.Flags().Bool("no-monorepo", true, "")
	case "uninstall":
		c.RunE = runUninstall
		c.Flags().Bool("keep-data", false, "")
		c.Flags().Bool("purge", false, "")
	}
	c.Flags().Bool("yes", false, "")
	c.Flags().Bool("json", false, "")
	c.SetContext(context.Background())
	c.SetIn(strings.NewReader(stdin))
	c.SetOut(io.Discard)
	c.SetErr(io.Discard)
	if err := c.ParseFlags(flags); err != nil {
		t.Fatal(err)
	}
	err := c.RunE(c, nil)
	return out.String(), err
}

func goldenFor(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "json", name+".golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBuildFragmentEnvelopes(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	noPrune := func(ctx context.Context, out, errOut io.Writer) error { return nil }
	orig := dockerSystemPrune
	dockerSystemPrune = noPrune
	t.Cleanup(func() { dockerSystemPrune = orig })

	cases := []struct {
		name  string
		flags []string
		want  string // golden name; empty means a data check only
	}{
		{"clean", []string{"--json"}, "clean"},
		{"reset", []string{"--json", "--yes"}, "reset"},
		{"uninstall", []string{"--json", "--yes"}, "uninstall"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := buildFixture(t)
			var out string
			inDir(t, dir, func() {
				var err error
				out, err = runBuildCmd(t, tc.name, "", tc.flags...)
				if err != nil {
					t.Fatal(err)
				}
			})
			// Schemas resolve relative to the package dir, so validate outside inDir.
			validateEnvelope(t, out, tc.name)
			if out != goldenFor(t, tc.want) {
				t.Fatalf("envelope differs from golden\n--- got ---\n%s\n--- want ---\n%s", out, goldenFor(t, tc.want))
			}
			if _, statErr := os.Stat(filepath.Join(dir, "docker-compose.yml")); !os.IsNotExist(statErr) {
				t.Fatal("docker-compose.yml was reported removed but still exists")
			}
			if _, statErr := os.Stat(filepath.Join(dir, ".env")); statErr != nil {
				t.Fatalf(".env must survive: %v", statErr)
			}
		})
	}
}

func TestBuildFragmentFlagVariants(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	var pruned int
	orig := dockerSystemPrune
	dockerSystemPrune = func(ctx context.Context, out, errOut io.Writer) error { pruned++; return nil }
	t.Cleanup(func() { dockerSystemPrune = orig })

	check := func(name string, want map[string]any, flags ...string) {
		t.Helper()
		var out string
		inDir(t, buildFixture(t), func() {
			var err error
			out, err = runBuildCmd(t, name, "", flags...)
			if err != nil {
				t.Fatal(err)
			}
		})
		data := validateEnvelope(t, out, name)
		for k, v := range want {
			if data[k] != v {
				t.Fatalf("%s %v: data[%s] = %v, want %v", name, flags, k, data[k], v)
			}
		}
	}
	check("clean", map[string]any{"host_prune": true}, "--json", "--all", "--yes")
	if pruned != 1 {
		t.Fatalf("host prune ran %d times, want 1", pruned)
	}
	check("reset", map[string]any{"kept_data": true}, "--json", "--yes", "--keep-data")
	check("reset", map[string]any{"kept_data": false}, "--json", "--yes", "--purge")
	check("uninstall", map[string]any{"kept_data": false}, "--json", "--yes", "--purge")
}

// A declined prompt in JSON mode is an error carrying destructive_blocked, with
// nothing removed, never an empty success envelope.
func TestBuildFragmentDeclinedIsError(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	var pruned int
	orig := dockerSystemPrune
	dockerSystemPrune = func(ctx context.Context, out, errOut io.Writer) error { pruned++; return nil }
	t.Cleanup(func() { dockerSystemPrune = orig })

	for _, tc := range []struct {
		name  string
		flags []string
	}{
		{"clean", []string{"--json", "--all"}},
		{"uninstall", []string{"--json"}},
		{"reset", []string{"--json", "--keep-data"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := buildFixture(t)
			inDir(t, dir, func() {
				out, err := runBuildCmd(t, tc.name, "no\n", tc.flags...)
				if !errors.Is(err, errs.ErrDestructiveBlocked) {
					t.Fatalf("err = %v, want destructive_blocked", err)
				}
				if out != "" {
					t.Fatalf("a refusal must not print a data envelope, got %q", out)
				}
				if _, statErr := os.Stat(filepath.Join(dir, ".nself", "volumes")); statErr != nil {
					t.Fatalf("data must survive a refusal: %v", statErr)
				}
			})
		})
	}
	if pruned != 0 {
		t.Fatalf("host prune ran %d times without confirmation", pruned)
	}
}

// v1.4 keeps ignoring --json: no envelope, same exit.
func TestBuildFragmentLegacyModeSilent(t *testing.T) {
	t.Setenv("NSELF_V15", "")
	for _, name := range []string{"clean", "reset", "uninstall"} {
		t.Run(name, func(t *testing.T) {
			inDir(t, buildFixture(t), func() {
				flags := []string{"--json"}
				if name != "clean" {
					flags = append(flags, "--yes")
				}
				out, err := runBuildCmd(t, name, "", flags...)
				if err != nil {
					t.Fatal(err)
				}
				if out != "" {
					t.Fatalf("v1.4 mode printed an envelope: %q", out)
				}
			})
		})
	}
}

func TestBuildFragmentConfirmRows(t *testing.T) {
	reg, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]cmdregistry.Command{}
	for _, c := range reg.Commands {
		by[c.Path] = c
	}
	b := by["nself build"]
	if b.Confirm == nil || b.Confirm.Plan == nil || b.Confirm.Plan.Flag != "plan" || b.Confirm.Plan.IDFlag != "plan-id" ||
		strings.Join(b.Confirm.Flags, ",") != "yes" {
		t.Fatalf("build confirm = %+v", b.Confirm)
	}
	for _, p := range []string{"nself reset", "nself clean", "nself uninstall"} {
		c := by[p]
		if c.Confirm == nil || strings.Join(c.Confirm.Flags, ",") != "yes" || c.Confirm.Plan != nil {
			t.Fatalf("%s confirm = %+v, want flags [yes]", p, c.Confirm)
		}
	}
	for _, p := range []string{"nself reset", "nself uninstall"} {
		if got := by[p].SideEffect; got != "destructive" {
			t.Fatalf("%s side_effect = %s", p, got)
		}
	}
	clean := by["nself clean"]
	if got := cmdregistry.EffectiveSideEffect(&clean, map[string]bool{}); got == "destructive" {
		t.Fatalf("plain clean must not be destructive, got %s", got)
	}
	if got := cmdregistry.EffectiveSideEffect(&clean, map[string]bool{"all": true}); got != "destructive" {
		t.Fatalf("clean --all effective = %s, want destructive", got)
	}
}
