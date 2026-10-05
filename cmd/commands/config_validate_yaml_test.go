package commands

// Purpose: tests for the nself.yaml step of "nself config validate"
// (P7-SURF-07): findings go to stderr, v1.4 only warns and keeps the exit
// status, v1.5 fails with the findings' codes.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

// runValidateWithManifest runs config validate in a fresh project whose
// nself.yaml is the named build fixture ("" = no manifest). It returns the
// error and what the command wrote to stderr.
// manifestFixtureDir is the absolute path of the build fixtures, resolved once
// before any test changes the working directory.
var manifestFixtureDir, _ = filepath.Abs(filepath.Join("..", "..", "internal", "build", "testdata", "manifest"))

func runValidateWithManifest(t *testing.T, fixture string) (error, string) {
	t.Helper()
	src := filepath.Join(manifestFixtureDir, fixture)
	dir := makeNSelfProject(t, "PROJECT_NAME=validatetest\nBASE_DOMAIN=validate.dev\n")
	if fixture != "" {
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "nself.yaml"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root := newConfigCmd()
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetArgs([]string{"config", "validate"})
	_, err := captureStdout(t, func() error { return root.Execute() })
	return err, stderr.String()
}

func TestConfigValidateNselfYAML(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		v15 := compat.V15()
		baseErr, baseStderr := runValidateWithManifest(t, "")
		if baseErr != nil {
			t.Fatalf("baseline project must validate (otherwise the exit checks prove nothing): %v\n%s", baseErr, baseStderr)
		}
		if strings.Contains(baseStderr, "nself.yaml") {
			t.Errorf("no manifest, no findings: %q", baseStderr)
		}

		// A clean manifest changes nothing.
		if err, se := runValidateWithManifest(t, "x-extensions.yaml"); err != nil || se != "" {
			t.Errorf("clean manifest: err=%v stderr=%q", err, se)
		}

		// The nclaw fixture: unknown project, and plugins.pro entries that are maps.
		err, se := runValidateWithManifest(t, "nclaw.yaml")
		for _, want := range []string{"nself.yaml:", "project", "[E436]", `did you mean "app"`, "plugins.pro[0]", "[E435]"} {
			if !strings.Contains(se, want) {
				t.Errorf("stderr missing %q:\n%s", want, se)
			}
		}
		if v15 {
			if err == nil || !strings.Contains(se, "error:") || strings.Contains(se, "warning:") {
				t.Errorf("v1.5 must fail with errors: err=%v\n%s", err, se)
			}
		} else if err != nil || !strings.Contains(se, "warning:") || strings.Contains(se, "error:") {
			t.Errorf("v1.4 must warn and keep the exit status: err=%v\n%s", err, se)
		}

		// plugins: 7 is warned about in v1.4 and fails with E435 in v1.5.
		err, se = runValidateWithManifest(t, "type-error.yaml")
		if !strings.Contains(se, "nself.yaml:2:") || !strings.Contains(se, "[E435]") {
			t.Errorf("type error finding missing:\n%s", se)
		}
		if (err != nil) != v15 {
			t.Errorf("plugins: 7 exit: err=%v v15=%v", err, v15)
		}
	})
}
