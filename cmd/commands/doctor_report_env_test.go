package commands

// Tests for the compose.env wiring of `nself doctor report`: the command
// passes build.ComposeEnvFiles to the report as a scrub input (CR r3 N-3) and
// to its docker compose reads (D-0539).

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/doctor/report"
)

func TestDoctorReportComposeEnvFileIsAScrubInput(t *testing.T) {
	proj, _ := reportFixture(t)
	only := "ce" + strings.Repeat("Vb7", 8)
	if err := os.MkdirAll(filepath.Join(proj, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".nself", "compose.env"), []byte("HASURA_GRAPHQL_ADMIN_SECRET="+only+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := doctorReportSources
	doctorReportSources = func(ds report.DockerSetup) report.Sources {
		s := old(ds)
		s.Logs = func(context.Context, int) (map[string]string, error) {
			return map[string]string{"hasura": "admin " + only + " rejected\n"}, nil
		}
		return s
	}
	t.Cleanup(func() { doctorReportSources = old })
	out := filepath.Join(proj, "ce.tar.gz")
	setReportFlags(t, map[string]string{"output": out, "log-lines": "5"})
	if err := runDoctorReport(doctorReportCmd, nil); err != nil {
		t.Fatal(err)
	}
	if _, all := archiveNames(t, out); strings.Contains(all, only) {
		t.Error("the compose.env-only secret reached the archive")
	}
}

// TestDoctorReportComposeReadsSeeComposeEnvOnlyVariable: the report's docker
// compose reads pass .nself/compose.env (D-0539). The stub fails like compose
// does on ${FAKE_ONLY_IN_COMPOSE_ENV:?} unless an --env-file defines it, and
// that variable is only in compose.env.
func TestDoctorReportComposeReadsSeeComposeEnvOnlyVariable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	proj, _ := reportFixture(t)
	if err := os.MkdirAll(filepath.Join(proj, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".nself", "compose.env"), []byte("FAKE_ONLY_IN_COMPOSE_ENV=synthetic\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "docker")
	stub := "#!/bin/sh\nfound=\nprev=\nfor a in \"$@\"; do\n" +
		"  if [ \"$prev\" = --env-file ] && grep -q '^FAKE_ONLY_IN_COMPOSE_ENV=' \"$a\"; then found=1; fi\n  prev=$a\ndone\n" +
		"[ -n \"$found\" ] || { echo 'required variable FAKE_ONLY_IN_COMPOSE_ENV is missing a value' >&2; exit 1; }\n" +
		"case \"$*\" in *'config --services'*) echo hasura ;; *' ps '*) echo '[]' ;; esac\n"
	if err := os.WriteFile(bin, []byte(stub), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}

	ds, _ := reportDockerSetup(proj)
	if want := build.ComposeEnvFiles(proj); !reflect.DeepEqual(ds.EnvFiles, want) {
		t.Fatalf("ds.EnvFiles = %q, want build.ComposeEnvFiles %q", ds.EnvFiles, want)
	}
	ds.DockerPath = bin
	if _, err := report.RealSources(ds).Logs(context.Background(), 5); err != nil {
		t.Errorf("log read with the project env files: %v", err)
	}
	// Positive control: without compose.env the same read fails.
	ds.EnvFiles = []string{filepath.Join(proj, ".env")}
	if _, err := report.RealSources(ds).Logs(context.Background(), 5); err == nil || !strings.Contains(err.Error(), "FAKE_ONLY_IN_COMPOSE_ENV") {
		t.Errorf("control: err = %v, want the missing-variable failure", err)
	}
}
