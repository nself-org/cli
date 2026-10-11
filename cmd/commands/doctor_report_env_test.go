package commands

// Test for the compose.env wiring of `nself doctor report` (CR r3 N-3): the
// command passes build.ComposeEnvFiles to the report as a scrub input.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
