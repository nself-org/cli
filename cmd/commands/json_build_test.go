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
	// Compose down is stubbed directly too: the PATH stub is a shell script
	// that Windows cannot run, and a failed down is now a JSON-mode error.
	origDown, origReset := composeDown, resetComposeDown
	composeDown = func(context.Context, io.Writer, io.Writer, string, bool) (bool, error) { return true, nil }
	resetComposeDown = func(context.Context, string) error { return nil }
	t.Cleanup(func() { composeDown, resetComposeDown = origDown, origReset })
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
// nothing removed (no generated artifact, no data), never an empty success
// envelope. clean --all used to delete its local artifacts before asking.
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
		{"reset", []string{"--json"}},
		{"reset", []string{"--json", "--keep-data"}},
	} {
		t.Run(tc.name+strings.Join(tc.flags, ""), func(t *testing.T) {
			dir := buildFixture(t)
			inDir(t, dir, func() {
				out, err := runBuildCmd(t, tc.name, "no\n", tc.flags...)
				if !errors.Is(err, errs.ErrDestructiveBlocked) {
					t.Fatalf("err = %v, want destructive_blocked", err)
				}
				if out != "" {
					t.Fatalf("a refusal must not print a data envelope, got %q", out)
				}
			})
			assertSurvives(t, dir, "docker-compose.yml", "nginx/sites/app.conf", ".nself/cache/build.json", ".nself/volumes/pg/data")
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

// assertSurvives fails unless every project-relative path still exists.
func assertSurvives(t *testing.T, dir string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s must survive: %v", rel, err)
		}
	}
}

// assertInfraFailure fails unless err is an infra-class (exit 2) failure
// whose message contains every want, and nothing was printed on stdout.
func assertInfraFailure(t *testing.T, out string, err error, want ...string) {
	t.Helper()
	if got := errs.ExitCodeFor(err); got != errs.ExitInfraError {
		t.Fatalf("exit = %d (err %v), want %d", got, err, errs.ExitInfraError)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Fatalf("error %q does not mention %q", err, w)
		}
	}
	if out != "" {
		t.Fatalf("a failure must not print a success envelope, got %q", out)
	}
}

// The purge gate takes only the word: "y" twice is a refusal, and a piped
// "y" then "purge" passes both prompts (each prompt reads its own line).
func TestBuildFragmentPurgeNeedsTheWord(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	dir := buildFixture(t)
	inDir(t, dir, func() {
		if _, err := runBuildCmd(t, "uninstall", "y\ny\n", "--json", "--purge"); !errors.Is(err, errs.ErrDestructiveBlocked) {
			t.Fatalf("err = %v, want destructive_blocked", err)
		}
	})
	assertSurvives(t, dir, "docker-compose.yml", ".nself/volumes/pg/data")

	var out string
	inDir(t, dir, func() {
		var err error
		if out, err = runBuildCmd(t, "uninstall", "y\npurge\n", "--json", "--purge"); err != nil {
			t.Fatal(err)
		}
	})
	if data := validateEnvelope(t, out, "uninstall"); data["kept_data"] != false {
		t.Fatalf("kept_data = %v after a confirmed purge", data["kept_data"])
	}
}

// alreadyRemoved returns the "already removed: ..." list of a partial-removal
// error, or "" when the message has none.
func alreadyRemoved(err error) string {
	msg := err.Error()
	i := strings.Index(msg, "already removed: ")
	if i < 0 {
		return ""
	}
	rest := msg[i+len("already removed: "):]
	if j := strings.Index(rest, "):"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// failRemoveAll makes removeAll fail with EPERM for paths whose base is base.
func failRemoveAll(t *testing.T, base string) {
	t.Helper()
	orig := removeAll
	removeAll = func(p string) error {
		if filepath.Base(p) == base {
			return os.ErrPermission
		}
		return os.RemoveAll(p)
	}
	t.Cleanup(func() { removeAll = orig })
}

// A path that failed to delete is never reported as removed; in JSON mode the
// failure is an infra error naming what was removed, the database volumes
// first when compose down --volumes already ran. Human mode keeps exit 0.
func TestBuildFragmentRemovalFailure(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	failRemoveAll(t, "volumes")

	for _, name := range []string{"uninstall", "reset"} {
		t.Run(name, func(t *testing.T) {
			dir := buildFixture(t)
			inDir(t, dir, func() {
				out, err := runBuildCmd(t, name, "", "--json", "--yes", "--purge")
				assertInfraFailure(t, out, err, "removing .nself/volumes/")
				got := alreadyRemoved(err)
				if !strings.HasPrefix(got, volumesGone+", docker-compose.yml") || strings.Contains(got, "volumes/") {
					t.Fatalf("already removed = %q: want the volumes first and never the failed path", got)
				}
			})
			assertSurvives(t, dir, ".nself/volumes/pg/data")
		})
	}
	inDir(t, buildFixture(t), func() {
		if _, err := runBuildCmd(t, "uninstall", "", "--yes", "--purge"); err != nil {
			t.Fatalf("human mode keeps its exit 0 and warning, got %v", err)
		}
	})
}

// Bare reset and clean fail JSON mode on a generated file they could not
// delete, instead of a success envelope that omits it.
func TestBuildFragmentGeneratedRemovalFailure(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	t.Run("reset", func(t *testing.T) {
		failRemoveAll(t, "app.conf")
		inDir(t, buildFixture(t), func() {
			out, err := runBuildCmd(t, "reset", "", "--json", "--yes")
			assertInfraFailure(t, out, err, "removing nginx/sites/app.conf")
			if got := alreadyRemoved(err); got != volumesGone+", docker-compose.yml, .env.computed" {
				t.Fatalf("already removed = %q", got)
			}
		})
	})
	t.Run("clean", func(t *testing.T) {
		failRemoveAll(t, "cache")
		var pruned int
		orig := dockerSystemPrune
		dockerSystemPrune = func(context.Context, io.Writer, io.Writer) error { pruned++; return nil }
		t.Cleanup(func() { dockerSystemPrune = orig })
		inDir(t, buildFixture(t), func() {
			out, err := runBuildCmd(t, "clean", "", "--json", "--all", "--yes")
			assertInfraFailure(t, out, err, "removing .nself/cache/")
			if got := alreadyRemoved(err); got != "docker-compose.yml, nginx/sites/app.conf" {
				t.Fatalf("already removed = %q", got)
			}
		})
		if pruned != 0 {
			t.Fatal("host prune ran after a failed local clean")
		}
	})
}

// --purge with no compose file deletes no volume, so kept_data stays true.
func TestBuildFragmentPurgeWithoutComposeKeepsData(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	for _, name := range []string{"reset", "uninstall"} {
		t.Run(name, func(t *testing.T) {
			dir := buildFixture(t)
			composeDown = runDockerComposeDown // real: no compose file, docker never runs
			if err := os.Remove(filepath.Join(dir, "docker-compose.yml")); err != nil {
				t.Fatal(err)
			}
			var out string
			inDir(t, dir, func() {
				var err error
				if out, err = runBuildCmd(t, name, "", "--json", "--yes", "--purge"); err != nil {
					t.Fatal(err)
				}
			})
			if data := validateEnvelope(t, out, name); data["kept_data"] != true {
				t.Fatalf("kept_data = %v, want true when no volume was deleted", data["kept_data"])
			}
		})
	}
}

// A failed compose down stops JSON mode before the compose file is deleted
// (the caller needs it to retry) and is never a success envelope.
func TestBuildFragmentComposeDownFailure(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	downErr := errors.New("daemon unreachable")
	for _, tc := range []struct {
		name  string
		flags []string
	}{
		{"reset", []string{"--json", "--yes"}},
		{"reset", []string{"--json", "--yes", "--purge"}},
		{"uninstall", []string{"--json", "--yes"}},
	} {
		t.Run(tc.name+strings.Join(tc.flags, ""), func(t *testing.T) {
			dir := buildFixture(t)
			composeDown = func(context.Context, io.Writer, io.Writer, string, bool) (bool, error) { return true, downErr }
			resetComposeDown = func(context.Context, string) error { return downErr }
			inDir(t, dir, func() {
				out, err := runBuildCmd(t, tc.name, "", tc.flags...)
				assertInfraFailure(t, out, err, "docker compose down", "no files were removed")
				if !errors.Is(err, downErr) {
					t.Fatalf("err %v does not wrap the down failure", err)
				}
			})
			assertSurvives(t, dir, "docker-compose.yml", "nginx/sites/app.conf")
		})
	}

	// Human mode warns and carries on, exactly as before.
	dir := buildFixture(t)
	resetComposeDown = func(context.Context, string) error { return downErr }
	inDir(t, dir, func() {
		if _, err := runBuildCmd(t, "reset", "", "--yes"); err != nil {
			t.Fatalf("human reset: %v", err)
		}
	})
	if _, err := os.Stat(filepath.Join(dir, "docker-compose.yml")); !os.IsNotExist(err) {
		t.Fatal("human reset should still remove docker-compose.yml")
	}
}

// Bare reset only reports kept_data false when compose down --volumes ran:
// with no compose file nothing touched the volumes.
func TestBuildFragmentResetWithoutComposeKeepsData(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	dir := buildFixture(t)
	if err := os.Remove(filepath.Join(dir, "docker-compose.yml")); err != nil {
		t.Fatal(err)
	}
	var out string
	inDir(t, dir, func() {
		var err error
		if out, err = runBuildCmd(t, "reset", "", "--json", "--yes"); err != nil {
			t.Fatal(err)
		}
	})
	if data := validateEnvelope(t, out, "reset"); data["kept_data"] != true {
		t.Fatalf("kept_data = %v, want true when no volumes were removed", data["kept_data"])
	}
}

// An explicitly requested host prune that fails is an infra error in JSON
// mode, naming the local artifacts that were already removed.
func TestBuildFragmentCleanPruneFailure(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	orig := dockerSystemPrune
	dockerSystemPrune = func(context.Context, io.Writer, io.Writer) error { return errors.New("prune refused") }
	t.Cleanup(func() { dockerSystemPrune = orig })

	inDir(t, buildFixture(t), func() {
		out, err := runBuildCmd(t, "clean", "", "--json", "--all", "--yes")
		assertInfraFailure(t, out, err, "docker system prune failed", "already removed: docker-compose.yml")
	})
	inDir(t, buildFixture(t), func() {
		if _, err := runBuildCmd(t, "clean", "", "--all", "--yes"); err != nil {
			t.Fatalf("human clean --all keeps its exit 0 and warning, got %v", err)
		}
	})
}

func TestReadAnswerOneLinePerCall(t *testing.T) {
	r := strings.NewReader("Y\r\n purge \nlast")
	for _, want := range []string{"y", "purge", "last"} {
		if got, ok := readAnswer(r); !ok || got != want {
			t.Fatalf("readAnswer = %q, %v; want %q", got, ok, want)
		}
	}
	if _, ok := readAnswer(r); ok {
		t.Fatal("readAnswer at EOF must report ok=false")
	}
	if _, ok := readAnswer(strings.NewReader(strings.Repeat("y", 2048) + "\n")); ok {
		t.Fatal("an over-long line must be a refusal")
	}
	if _, ok := readAnswer(stallReader{}); ok {
		t.Fatal("a reader that never returns data must be a refusal")
	}
}

// stallReader returns (0, nil) forever.
type stallReader struct{}

func (stallReader) Read([]byte) (int, error) { return 0, nil }
