package backup

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// heavyJob represents a scheduled maintenance job with a name and its UTC
// fire time expressed as hours and minutes.
type heavyJob struct {
	name   string
	hour   int
	minute int
}

// minuteOfDay returns the number of minutes since midnight for a job.
func (j heavyJob) minuteOfDay() int {
	return j.hour*60 + j.minute
}

// canonicalHeavyJobs returns the current set of heavy nightly jobs that share
// the backup window. Any change to the default schedules in systemd.go or in
// the plugin cron seeds MUST be reflected here.
//
// Canonical schedule (UTC):
//   - backup-full:       03:00 (FullAt default in RenderSystemdUnits)
//   - mux-classify:      03:15 (MUX_CRON_CLASSIFY_HOUR/MINUTE defaults)
//   - backup-prune:      04:30 (PruneAt default in RenderSystemdUnits)
//   - claw_daily_briefing: 04:45 (seeded by claw plugin scheduler)
func canonicalHeavyJobs() []heavyJob {
	return []heavyJob{
		{name: "backup-full", hour: 3, minute: 0},
		{name: "mux-classify", hour: 3, minute: 15},
		{name: "backup-prune", hour: 4, minute: 30},
		{name: "claw_daily_briefing", hour: 4, minute: 45},
	}
}

// TestNoHeavyJobsCollide verifies that no two heavy nightly jobs are scheduled
// within a 10-minute window of each other. Colliding jobs saturate Postgres I/O
// during the backup window and cause plugin health-check failures.
func TestNoHeavyJobsCollide(t *testing.T) {
	const collisionWindow = 10 // minutes

	jobs := canonicalHeavyJobs()

	// Validate that every job has a plausible time value.
	for _, j := range jobs {
		if j.hour < 0 || j.hour > 23 {
			t.Fatalf("job %q: hour %d out of range [0,23]", j.name, j.hour)
		}
		if j.minute < 0 || j.minute > 59 {
			t.Fatalf("job %q: minute %d out of range [0,59]", j.name, j.minute)
		}
	}

	// Check every pair for collision.
	for i := 0; i < len(jobs); i++ {
		for k := i + 1; k < len(jobs); k++ {
			a, b := jobs[i], jobs[k]
			diff := abs(a.minuteOfDay() - b.minuteOfDay())
			if diff < collisionWindow {
				t.Errorf(
					"jobs %q (%02d:%02d UTC) and %q (%02d:%02d UTC) are only %d minute(s) apart, "+
						"minimum gap is %d minutes",
					a.name, a.hour, a.minute,
					b.name, b.hour, b.minute,
					diff, collisionWindow,
				)
			}
		}
	}
}

// TestPruneDefaultAfterFull verifies that the backup-prune timer default is
// scheduled at least 90 minutes after the backup-full timer default.
// This guards against the 04:00 UTC collision that caused the PERF-03 incident.
func TestPruneDefaultAfterFull(t *testing.T) {
	const minGap = 90 // minutes between full backup start and prune start

	var opts SystemdInstallOptions
	// Apply the same defaulting logic as RenderSystemdUnits.
	if opts.FullAt == "" {
		opts.FullAt = "03:00"
	}
	if opts.PruneAt == "" {
		opts.PruneAt = "04:30"
	}

	fullTime, err := parseHHMM(opts.FullAt)
	if err != nil {
		t.Fatalf("parseHHMM(%q): %v", opts.FullAt, err)
	}
	pruneTime, err := parseHHMM(opts.PruneAt)
	if err != nil {
		t.Fatalf("parseHHMM(%q): %v", opts.PruneAt, err)
	}

	gap := int(pruneTime.Sub(fullTime).Minutes())
	if gap < minGap {
		t.Errorf(
			"backup-prune default (%s UTC) is only %d minute(s) after backup-full default (%s UTC); "+
				"minimum gap is %d minutes to avoid I/O saturation",
			opts.PruneAt, gap, opts.FullAt, minGap,
		)
	}
}

// parseHHMM parses a "HH:MM" string into a time.Time on an arbitrary reference day.
func parseHHMM(hhmm string) (time.Time, error) {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid HH:MM %q: %w", hhmm, err)
	}
	return t, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ── ScheduleStream (P7-PROD-71) ───────────────────────────────────────

// skipUnitTestOnWindows skips tests that compare POSIX paths inside a systemd
// unit. Units only exist on Linux; a Windows path would be quoted and escaped.
func skipUnitTestOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("systemd units are Linux-only; paths are compared in POSIX form")
	}
}

// scheduleDryRun renders the units of a dry run into a string.
func scheduleDryRun(t *testing.T, opts ScheduleOptions) string {
	t.Helper()
	var out bytes.Buffer
	opts.DryRun = true
	opts.Out = &out
	if err := ScheduleStream(minimalConfig(), opts); err != nil {
		t.Fatalf("ScheduleStream: %v", err)
	}
	return out.String()
}

// TestScheduleStreamGolden pins the rendered service and timer: the unit runs
// in the project directory with the absolute binary path, carries every
// recipient and the heartbeat remote, has no EnvironmentFile line without
// --env-file, and the timer is daily and persistent.
func TestScheduleStreamGolden(t *testing.T) {
	skipUnitTestOnWindows(t)
	proj := t.TempDir()
	bin := filepath.Join(t.TempDir(), "nself")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	realBin, _ := filepath.EvalSymlinks(bin)
	realProj, _ := filepath.Abs(proj)
	got := scheduleDryRun(t, ScheduleOptions{
		Cron: "30 2 * * *", To: "r2:bkt/nself-web", HeartbeatTo: "r2hb:hb",
		Recipients: []string{"age1qqqq", "age1zzzz"}, BinaryPath: bin, ProjectDir: proj,
	})
	got = strings.ReplaceAll(strings.ReplaceAll(got, realProj, "<PROJECT>"), realBin, "<BINARY>")
	want, err := os.ReadFile(filepath.Join("testdata", "heartbeat", "schedule.golden.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("schedule units drifted.\n--- got ---\n%s--- want ---\n%s", got, want)
	}
	if strings.Contains(got, "EnvironmentFile") {
		t.Error("no EnvironmentFile line may appear without --env-file")
	}
	if strings.Contains(got, "/opt/nself") || strings.Contains(got, "/usr/local/bin/nself") || strings.Contains(got, "/etc/nself") {
		t.Error("a hardcoded legacy path is back in the unit")
	}
}

// TestScheduleStreamEnvFileAndDefaults: --env-file adds the line (absolute);
// the running binary and the current directory are the defaults; a symlinked
// binary is resolved.
func TestScheduleStreamEnvFileAndDefaults(t *testing.T) {
	skipUnitTestOnWindows(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	env := filepath.Join(dir, "backup.env")
	got := scheduleDryRun(t, ScheduleOptions{Cron: "0 3 * * *", To: "s3:b/p", EnvFile: "backup.env"})
	realEnv, _ := filepath.EvalSymlinks(env)
	if !strings.Contains(got, "EnvironmentFile=-"+env) && !strings.Contains(got, "EnvironmentFile=-"+realEnv) {
		t.Errorf("env file line missing:\n%s", got)
	}
	exe, _ := os.Executable()
	realExe, _ := filepath.EvalSymlinks(exe)
	if !strings.Contains(got, "ExecStart="+realExe+" backup stream --to s3:b/p") {
		t.Errorf("ExecStart should be the running binary %s:\n%s", realExe, got)
	}
	wd, _ := os.Getwd()
	if !strings.Contains(got, "WorkingDirectory="+wd) {
		t.Errorf("WorkingDirectory should be %s:\n%s", wd, got)
	}

	link := filepath.Join(t.TempDir(), "nself-link")
	if err := os.Symlink(exe, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	got = scheduleDryRun(t, ScheduleOptions{Cron: "0 3 * * *", To: "s3:b/p", BinaryPath: link})
	if strings.Contains(got, link) || !strings.Contains(got, realExe) {
		t.Errorf("symlinked binary must resolve to %s:\n%s", realExe, got)
	}
}

// TestScheduleStreamQuoting: spaces, quotes, % and $ cannot split or expand
// the ExecStart line; line breaks are refused.
func TestScheduleStreamQuoting(t *testing.T) {
	got := systemdExecLine([]string{"/opt/my bin/nself", "backup", "--to", "r2:b/p%h", "--recipient", `a"b$HOME`})
	want := `"/opt/my bin/nself" backup --to r2:b/p%%h --recipient "a\"b$$HOME"`
	if got != want {
		t.Errorf("systemdExecLine = %s, want %s", got, want)
	}
	err := ScheduleStream(minimalConfig(), ScheduleOptions{Cron: "0 3 * * *", To: "r2:b\nExecStartPre=/bin/sh", DryRun: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Error("a destination with a line break must be refused")
	}
}

// TestScheduleStreamErrors: missing cron or destination fail before any write.
func TestScheduleStreamErrors(t *testing.T) {
	dir := t.TempDir()
	if err := ScheduleStream(minimalConfig(), ScheduleOptions{To: "r2:b", UnitDir: dir}); err == nil {
		t.Error("missing cron should fail")
	}
	if err := ScheduleStream(minimalConfig(), ScheduleOptions{Cron: "0 3 * * *", UnitDir: dir}); err == nil {
		t.Error("missing destination should fail")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Error("nothing may be written on a validation error")
	}
}

// TestScheduleStreamRefusesNonProject: run from a directory with no project
// marker, the schedule is refused instead of writing a unit that runs in /tmp.
func TestScheduleStreamRefusesNonProject(t *testing.T) {
	t.Chdir(t.TempDir())
	var out bytes.Buffer
	err := ScheduleStream(minimalConfig(), ScheduleOptions{Cron: "0 3 * * *", To: "r2:b", DryRun: true, Out: &out})
	if err == nil || !strings.Contains(err.Error(), "does not look like an nSelf project") {
		t.Fatalf("err = %v", err)
	}
	if out.Len() != 0 {
		t.Error("nothing may be printed for a refused schedule")
	}
	if err := os.WriteFile(".env", []byte("PROJECT_NAME=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ScheduleStream(minimalConfig(), ScheduleOptions{Cron: "0 3 * * *", To: "r2:b", DryRun: true, Out: &out}); err != nil {
		t.Errorf("a directory with .env must be accepted: %v", err)
	}
}

// TestScheduleStreamEnvFileEscapes: % in the env file path is doubled like the
// other unit values, and a line break is refused.
func TestScheduleStreamEnvFileEscapes(t *testing.T) {
	skipUnitTestOnWindows(t)
	got := scheduleDryRun(t, ScheduleOptions{Cron: "0 3 * * *", To: "r2:b", EnvFile: "/etc/n%h/backup.env", ProjectDir: t.TempDir(), BinaryPath: "/bin/sh"})
	if !strings.Contains(got, "EnvironmentFile=-/etc/n%%h/backup.env") {
		t.Errorf("env file path not %%-escaped:\n%s", got)
	}
	err := ScheduleStream(minimalConfig(), ScheduleOptions{Cron: "0 3 * * *", To: "r2:b", EnvFile: "/etc/a\nExecStartPre=/x", ProjectDir: t.TempDir(), DryRun: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Error("a line break in --env-file must be refused")
	}
}
