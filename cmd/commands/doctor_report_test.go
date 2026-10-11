package commands

// Tests for `nself doctor report`: the --json envelope in both compat modes,
// the human output, and the refusals. Docker and the doctor child are replaced
// through doctorReportSources; fake secrets are built at run time.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/doctor/report"
)

// reportFixture makes a project with one planted secret, swaps the sources,
// and returns the project dir and the secret.
func reportFixture(t *testing.T) (proj, secret string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // never read the real ~/.nself licence key
	proj = t.TempDir()
	secret = "cmd" + strings.Repeat("Rw5", 8)
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=proj\nENV=dev\nPOSTGRES_DB=myapp\nADMIN_SECRET="+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := doctorReportSources
	doctorReportSources = func(report.DockerSetup) report.Sources {
		return report.Sources{
			Versions: func(context.Context) map[string]string { return map[string]string{"nself": "test"} },
			Doctor:   func(context.Context) (string, error) { return "PASS ok " + secret + "\n", nil },
			Logs: func(context.Context, int) (map[string]string, error) {
				return map[string]string{"hasura": "denied: " + secret + "\n"}, nil
			},
			Now: func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) },
		}
	}
	t.Cleanup(func() { doctorReportSources = old })
	t.Chdir(proj)
	return proj, secret
}

// setReportFlags sets flags on doctorReportCmd and restores them afterwards.
func setReportFlags(t *testing.T, kv map[string]string) {
	t.Helper()
	doctorReportCmd.InheritedFlags()
	for k, v := range kv {
		f := doctorReportCmd.Flags().Lookup(k)
		if f == nil {
			t.Fatalf("flag --%s missing", k)
		}
		old := f.Value.String()
		if err := doctorReportCmd.Flags().Set(k, v); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = doctorReportCmd.Flags().Set(k, old); f.Changed = false })
	}
}

func archiveNames(t *testing.T, path string) (names []string, all string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return names, all
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		names = append(names, h.Name)
		all += string(b)
	}
}

func TestDoctorReportJSON(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		proj, secret := reportFixture(t)
		out := filepath.Join(proj, "bundle.tar.gz")
		stdout, _ := pilotBuffers(t)
		setReportFlags(t, map[string]string{"json": "true", "output": out, "log-lines": "5"})
		if err := runDoctorReport(doctorReportCmd, nil); err != nil {
			t.Fatal(err)
		}
		t.Chdir(testPkgDir) // the schema loader reads ../../schemas
		data := validateEnvelope(t, stdout.String(), "doctor report")
		if data["path"] != out {
			t.Errorf("path = %v, want %s", data["path"], out)
		}
		names, all := archiveNames(t, out)
		if int(data["files"].(float64)) != len(names) || data["bytes"].(float64) <= 0 || data["redactions"].(float64) < 1 {
			t.Errorf("data = %v, archive has %d files", data, len(names))
		}
		if strings.Contains(all, secret) || strings.Contains(stdout.String(), secret) {
			t.Error("the planted secret reached the archive or stdout")
		}
		if runtime.GOOS != "windows" {
			if st, _ := os.Stat(out); st.Mode().Perm() != 0o600 {
				t.Errorf("mode = %v, want 0600", st.Mode().Perm())
			}
		}
	})
}

func TestDoctorReportHuman(t *testing.T) {
	proj, _ := reportFixture(t)
	setReportFlags(t, map[string]string{"log-lines": "5"})
	var buf bytes.Buffer
	doctorReportCmd.SetOut(&buf)
	t.Cleanup(func() { doctorReportCmd.SetOut(nil) })
	if err := runDoctorReport(doctorReportCmd, nil); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(proj)
	got := strings.TrimSpace(buf.String())
	if filepath.Base(got) != "nself-report-20261010T120000Z.tar.gz" || !strings.HasPrefix(got, proj) && !strings.HasPrefix(got, want) {
		t.Errorf("printed %q", got)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("archive missing: %v", err)
	}
	// A second run in the same second must refuse, not overwrite.
	if err := runDoctorReport(doctorReportCmd, nil); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second run: %v", err)
	}
}

func TestDoctorReportRefusesBadLogLines(t *testing.T) {
	reportFixture(t)
	for _, v := range []string{"0", "-3", "100000"} {
		setReportFlags(t, map[string]string{"log-lines": v})
		if err := runDoctorReport(doctorReportCmd, nil); err == nil {
			t.Errorf("--log-lines %s accepted", v)
		}
	}
}

// The default archive lands in the project directory, not the working directory.
func TestDoctorReportDefaultOutputInProjectDir(t *testing.T) {
	proj, _ := reportFixture(t)
	sub := filepath.Join(proj, "deep", "er")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	setReportFlags(t, map[string]string{"log-lines": "5"})
	var buf bytes.Buffer
	doctorReportCmd.SetOut(&buf)
	t.Cleanup(func() { doctorReportCmd.SetOut(nil) })
	if err := runDoctorReport(doctorReportCmd, nil); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	wantDir, _ := filepath.EvalSymlinks(proj)
	gotDir, _ := filepath.EvalSymlinks(filepath.Dir(got))
	if gotDir != wantDir {
		t.Errorf("archive written to %s, want the project directory %s", gotDir, wantDir)
	}
}

// A compose fragment listed in .nself/compose-files.txt is a scrub input, and
// the archive always carries a logs/ directory entry.
func TestDoctorReportFragmentAndLogsDir(t *testing.T) {
	proj, _ := reportFixture(t)
	frag := "frg" + strings.Repeat("Tz6", 7)
	fragPath := filepath.Join(proj, "plugin-compose.yml")
	if err := os.WriteFile(fragPath, []byte("services:\n  p:\n    environment:\n      PLUGIN_TOKEN: "+frag+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".nself", "compose-files.txt"), []byte(fragPath+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := doctorReportSources
	doctorReportSources = func(ds report.DockerSetup) report.Sources {
		s := old(ds)
		s.Logs = func(context.Context, int) (map[string]string, error) {
			return map[string]string{"p": "plugin booted with " + frag + "\n"}, nil
		}
		return s
	}
	t.Cleanup(func() { doctorReportSources = old })
	out := filepath.Join(proj, "frag.tar.gz")
	setReportFlags(t, map[string]string{"output": out, "log-lines": "5"})
	if err := runDoctorReport(doctorReportCmd, nil); err != nil {
		t.Fatal(err)
	}
	names, all := archiveNames(t, out)
	if strings.Contains(all, frag) {
		t.Error("a fragment-only secret reached the archive")
	}
	hasDir := false
	for _, n := range names {
		hasDir = hasDir || n == "logs/"
	}
	if !hasDir {
		t.Errorf("no logs/ entry in %v", names)
	}
}

// S-4: an unreadable compose manifest is a manifest note, not silence. A
// directory in place of .nself/compose-files.txt cannot be read as a file.
func TestDoctorReportUnreadableComposeManifestIsANote(t *testing.T) {
	proj, _ := reportFixture(t)
	if err := os.MkdirAll(filepath.Join(proj, ".nself", "compose-files.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(proj, "note.tar.gz")
	setReportFlags(t, map[string]string{"output": out, "log-lines": "5"})
	if err := runDoctorReport(doctorReportCmd, nil); err != nil {
		t.Fatal(err)
	}
	_, all := archiveNames(t, out)
	if !strings.Contains(all, "compose manifest unreadable") {
		t.Error("manifest.json has no note about the unreadable compose manifest")
	}
}
