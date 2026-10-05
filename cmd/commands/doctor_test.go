package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/spf13/cobra"
)

// doctorJSONOutcome checks the exit status printDoctorJSON requested for a
// report and returns the report from stdout. v1.4: nil error, bare report.
// v1.5 (P7-REG-09): the report sits in the envelope `data` and a failed or
// warning report requests the state exit code (10 failed, 12 warnings only).
func doctorJSONOutcome(t *testing.T, report *doctorReport, wantV15Code int) doctorReport {
	t.Helper()
	out, err := captureDoctorStdout(t, func() error { return printDoctorJSON(report) })
	var decoded doctorReport
	if !compat.V15() {
		if err != nil {
			t.Fatalf("printDoctorJSON: %v", err)
		}
		if jerr := json.Unmarshal([]byte(out), &decoded); jerr != nil {
			t.Fatalf("output is not valid JSON: %v\nraw: %s", jerr, out)
		}
		return decoded
	}
	got := 0
	var ec *plugin.ExitCodeError
	if errors.As(err, &ec) {
		got = ec.Code
	} else if err != nil {
		t.Fatalf("printDoctorJSON: %v", err)
	}
	if got != wantV15Code {
		t.Fatalf("v1.5 exit code = %d, want %d", got, wantV15Code)
	}
	var env struct {
		Data doctorReport `json:"data"`
	}
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", jerr, out)
	}
	return env.Data
}

// newDoctorCmd returns a fresh cobra.Command tree with just the doctor command
// so tests can run in isolation without side effects from global RootCmd state.
func newDoctorCmd() *cobra.Command {
	root := &cobra.Command{Use: "nself", RunE: func(cmd *cobra.Command, args []string) error { return nil }}

	dc := &cobra.Command{
		Use:   "doctor",
		Short: "Run system diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Minimal stub to exercise flag wiring only; the full RunE in
			// doctor.go performs I/O that makes it unusable from tests
			// lacking Docker. Flag acceptance is what we care about here.
			return nil
		},
	}
	dc.Flags().BoolP("verbose", "v", false, "")
	dc.Flags().Bool("full", false, "")
	dc.Flags().Bool("deep", false, "")
	dc.Flags().Bool("fix", false, "")
	dc.Flags().Bool("json", false, "")
	dc.Flags().String("format", "", "")
	dc.Flags().String("only", "", "")
	dc.Flags().Bool("ai", false, "")
	dc.Flags().Bool("yes", false, "")
	dc.Flags().Bool("skip-ollama", false, "")
	dc.Flags().Bool("skip-pool", false, "")
	dc.Flags().Bool("headless", false, "")

	root.AddCommand(dc)
	return root
}

// TestDoctorCmd_Registered verifies doctor is registered on the global root.
func TestDoctorCmd_Registered(t *testing.T) {
	found := false
	for _, c := range RootCmd.Commands() {
		if c.Name() == "doctor" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'doctor' to be registered on RootCmd")
	}
}

// TestDoctorCmd_FlagJSON verifies --json is accepted.
func TestDoctorCmd_FlagJSON(t *testing.T) {
	root := newDoctorCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"doctor", "--json"})
	if err := root.Execute(); err != nil && strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--json not recognized: %v", err)
	}
}

// TestDoctorCmd_FlagVerbose verifies --verbose is accepted.
func TestDoctorCmd_FlagVerbose(t *testing.T) {
	root := newDoctorCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"doctor", "--verbose"})
	if err := root.Execute(); err != nil && strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--verbose not recognized: %v", err)
	}
}

// TestDoctorCmd_FlagOnly verifies --only=section is accepted.
func TestDoctorCmd_FlagOnly(t *testing.T) {
	root := newDoctorCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"doctor", "--only", "containers"})
	if err := root.Execute(); err != nil && strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--only not recognized: %v", err)
	}
}

// TestDoctorCmd_QuickFlagRegistered is a regression guard: `nself doctor
// --quick` was documented as a "Fast 10-second check" (help_topics.go) and
// relied on by scripts/golden-path.sh's health-wait loop (E2E golden-path
// smoke, steps 6-7), but --quick was never registered as a flag on the real
// doctorCmd — only the disconnected stub above (newDoctorCmd, which hand-lists
// its own flags rather than reading the real registration) accepted it.
// Every real invocation failed at cobra's flag parser with "unknown flag:
// --quick" before RunE ever ran, so the smoke's wait loop could never
// succeed regardless of actual service health — it was failing on flag
// parsing, not container state. This checks the actual flag set doctorCmd
// registers (not a copy), so a future removal of --quick fails a fast unit
// test instead of a weekly scheduled CI run.
func TestDoctorCmd_QuickFlagRegistered(t *testing.T) {
	f := doctorCmd.Flags().Lookup("quick")
	if f == nil {
		t.Fatal("doctor command has no --quick flag: `nself doctor --quick` " +
			"(documented in help_topics.go, used by scripts/golden-path.sh) " +
			"would fail with \"unknown flag: --quick\"")
	}
	if f.Value.Type() != "bool" {
		t.Fatalf("--quick should be a bool flag, got %q", f.Value.Type())
	}
}

// TestDoctorCmd_UnknownFlagRejected verifies unknown flags fail fast.
func TestDoctorCmd_UnknownFlagRejected(t *testing.T) {
	root := newDoctorCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"doctor", "--no-such-flag"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

// TestBuildDoctorReport_AllPass verifies summary aggregation when all pass.
func TestBuildDoctorReport_AllPass(t *testing.T) {
	checks := []doctorCheckResult{
		{Name: "c1", Status: "pass", Message: "ok"},
		{Name: "c2", Status: "pass", Message: "ok"},
		{Name: "c3", Status: "pass", Message: "ok"},
	}
	r := buildDoctorReport(checks)
	if r.Summary.Total != 3 {
		t.Errorf("Total = %d, want 3", r.Summary.Total)
	}
	if r.Summary.Passed != 3 {
		t.Errorf("Passed = %d, want 3", r.Summary.Passed)
	}
	if r.Summary.Warnings != 0 {
		t.Errorf("Warnings = %d, want 0", r.Summary.Warnings)
	}
	if r.Summary.Failed != 0 {
		t.Errorf("Failed = %d, want 0", r.Summary.Failed)
	}
	if r.Timestamp == "" {
		t.Error("Timestamp should be set")
	}
}

// TestBuildDoctorReport_OneFail verifies a single failing check increments Failed.
func TestBuildDoctorReport_OneFail(t *testing.T) {
	checks := []doctorCheckResult{
		{Name: "c1", Status: "pass", Message: "ok"},
		{Name: "c2", Status: "fail", Message: "broken"},
		{Name: "c3", Status: "pass", Message: "ok"},
	}
	r := buildDoctorReport(checks)
	if r.Summary.Total != 3 {
		t.Errorf("Total = %d, want 3", r.Summary.Total)
	}
	if r.Summary.Passed != 2 {
		t.Errorf("Passed = %d, want 2", r.Summary.Passed)
	}
	if r.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1", r.Summary.Failed)
	}
}

// TestBuildDoctorReport_MultipleFails verifies multiple failures sum correctly.
func TestBuildDoctorReport_MultipleFails(t *testing.T) {
	checks := []doctorCheckResult{
		{Name: "c1", Status: "fail", Message: "a"},
		{Name: "c2", Status: "fail", Message: "b"},
		{Name: "c3", Status: "warn", Message: "c"},
		{Name: "c4", Status: "pass", Message: "d"},
	}
	r := buildDoctorReport(checks)
	if r.Summary.Total != 4 {
		t.Errorf("Total = %d, want 4", r.Summary.Total)
	}
	if r.Summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1", r.Summary.Passed)
	}
	if r.Summary.Warnings != 1 {
		t.Errorf("Warnings = %d, want 1", r.Summary.Warnings)
	}
	if r.Summary.Failed != 2 {
		t.Errorf("Failed = %d, want 2", r.Summary.Failed)
	}
}

// TestBuildDoctorReport_WarningsOnly verifies warnings-only report.
func TestBuildDoctorReport_WarningsOnly(t *testing.T) {
	checks := []doctorCheckResult{
		{Name: "c1", Status: "pass", Message: "ok"},
		{Name: "c2", Status: "warn", Message: "meh"},
		{Name: "c3", Status: "warn", Message: "meh"},
	}
	r := buildDoctorReport(checks)
	if r.Summary.Failed != 0 {
		t.Errorf("Failed = %d, want 0", r.Summary.Failed)
	}
	if r.Summary.Warnings != 2 {
		t.Errorf("Warnings = %d, want 2", r.Summary.Warnings)
	}
}

// TestBuildDoctorReport_EmptyInput verifies zero-check input produces zero counts.
func TestBuildDoctorReport_EmptyInput(t *testing.T) {
	r := buildDoctorReport(nil)
	if r.Summary.Total != 0 || r.Summary.Passed != 0 || r.Summary.Warnings != 0 || r.Summary.Failed != 0 {
		t.Errorf("expected all zeros, got %+v", r.Summary)
	}
	if r.Timestamp == "" {
		t.Error("Timestamp should still be set on empty input")
	}
}

// TestBuildDoctorReport_UnknownStatusIgnored verifies unknown statuses only
// count toward Total and not toward passed/warn/fail.
func TestBuildDoctorReport_UnknownStatusIgnored(t *testing.T) {
	checks := []doctorCheckResult{
		{Name: "c1", Status: "pass", Message: "ok"},
		{Name: "c2", Status: "unknown", Message: "???"},
	}
	r := buildDoctorReport(checks)
	if r.Summary.Total != 2 {
		t.Errorf("Total = %d, want 2", r.Summary.Total)
	}
	sum := r.Summary.Passed + r.Summary.Warnings + r.Summary.Failed
	if sum != 1 {
		t.Errorf("pass+warn+fail sum = %d, want 1 (unknown ignored)", sum)
	}
}

// captureDoctorStdout redirects os.Stdout for the duration of f and returns
// what was written. Used to verify JSON output from printDoctorJSON.
func captureDoctorStdout(t *testing.T, f func() error) (string, error) {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	done := make(chan string)
	go func() {
		buf, _ := io.ReadAll(r)
		done <- string(buf)
	}()

	ferr := f()
	_ = w.Close()
	out := <-done
	return out, ferr
}

// TestPrintDoctorJSON_Valid verifies the JSON output matches the expected shape
// and that Summary counts round-trip correctly.
func TestPrintDoctorJSON_Valid(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		checks := []doctorCheckResult{
			{Name: "docker", Status: "pass", Message: "running"},
			{Name: "ports", Status: "warn", Message: "3000 in use"},
			{Name: "memory", Status: "fail", Message: "low"},
		}
		decoded := doctorJSONOutcome(t, buildDoctorReport(checks), 10)
		if decoded.Summary.Total != 3 {
			t.Errorf("decoded Total = %d, want 3", decoded.Summary.Total)
		}
		if decoded.Summary.Failed != 1 {
			t.Errorf("decoded Failed = %d, want 1", decoded.Summary.Failed)
		}
		if len(decoded.Checks) != 3 {
			t.Errorf("decoded %d checks, want 3", len(decoded.Checks))
		}
	})
}

// TestPrintDoctorJSON_EmptyChecks verifies the JSON output is valid even when
// there are zero checks.
func TestPrintDoctorJSON_EmptyChecks(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		decoded := doctorJSONOutcome(t, buildDoctorReport(nil), 0)
		if decoded.Summary.Total != 0 {
			t.Errorf("expected total=0, got %d", decoded.Summary.Total)
		}
		if compat.V15() && decoded.Checks == nil {
			t.Error("v1.5 data.checks must be an array, got null")
		}
	})
}

// TestCheckEnvExists_Missing verifies the check fails when no env file is present.
func TestCheckEnvExists_Missing(t *testing.T) {
	dir := t.TempDir()
	got := checkEnvExists(dir, false)
	if got.Status != "fail" {
		t.Errorf("status = %q, want fail", got.Status)
	}
	if !strings.Contains(got.Message, "no .env") {
		t.Errorf("message = %q, want contains 'no .env'", got.Message)
	}
}

// TestCheckEnvExists_FindsDotEnv verifies the check passes for .env.
func TestCheckEnvExists_FindsDotEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=x\n"), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	got := checkEnvExists(dir, false)
	if got.Status != "pass" {
		t.Errorf("status = %q, want pass", got.Status)
	}
	if !strings.Contains(got.Detail, ".env") {
		t.Errorf("detail should include .env path, got: %q", got.Detail)
	}
}

// TestCheckEnvExists_FindsDotEnvDev verifies the check passes for .env.dev when
// .env is missing.
func TestCheckEnvExists_FindsDotEnvDev(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.dev"), []byte("PROJECT_NAME=x\n"), 0600); err != nil {
		t.Fatalf("writing .env.dev: %v", err)
	}
	got := checkEnvExists(dir, false)
	if got.Status != "pass" {
		t.Errorf("status = %q, want pass", got.Status)
	}
	if !strings.Contains(got.Message, ".env.dev") {
		t.Errorf("message should mention .env.dev, got: %q", got.Message)
	}
}

// TestIsWeakPassword_DetectsBadPatterns verifies the weak-password detector
// flags known insecure substrings.
func TestIsWeakPassword_DetectsBadPatterns(t *testing.T) {
	weak := []string{
		"Password123",
		"ChangeMe!",
		"mysecret",
		"defaultadmin",
		"postgres-local",
		"12345abc",
	}
	for _, w := range weak {
		if !isWeakPassword(w) {
			t.Errorf("isWeakPassword(%q) = false, want true", w)
		}
	}
}

// TestIsWeakPassword_AcceptsStrongPasswords verifies that high-entropy strings
// are not flagged as weak.
func TestIsWeakPassword_AcceptsStrongPasswords(t *testing.T) {
	strong := []string{
		"XZ8q2fLm-vn4!KpR",
		"7g$Hn2v-4Qx9BmCd",
		"randomish-phrase-that-is-long-enough",
	}
	for _, s := range strong {
		if isWeakPassword(s) {
			t.Errorf("isWeakPassword(%q) = true, want false", s)
		}
	}
}
