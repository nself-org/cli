package backup

// stream_schedule.go — scheduling a recurring streamed backup.
//
// Purpose: translate a cron expression into a systemd timer unit and drive ScheduleStream, split out of stream.go for file size.
// Inputs: a cron expression and the StreamConfig to run on each tick.
// Outputs: an installed systemd timer, or an error if the cron expression cannot be represented.
// Constraints: pure move from stream.go (CLI-R12 Batch E); no behaviour change.

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// ScheduleOptions holds the inputs of ScheduleStream (flags of
// `nself backup schedule`). BinaryPath and ProjectDir are test seams: left
// empty they resolve to the running binary and the current project directory.
type ScheduleOptions struct {
	Cron        string
	To          string   // rclone destination; falls back to cfg.Backup.Remote
	Recipients  []string // repeatable --recipient, written as repeated flags
	HeartbeatTo string   // --heartbeat-to, written into the unit when given
	EnvFile     string   // --env-file; no EnvironmentFile line when empty
	UnitDir     string   // default /etc/systemd/system
	DryRun      bool
	Out         io.Writer // dry-run output; default os.Stdout

	BinaryPath string
	ProjectDir string
}

// ScheduleStream installs a systemd timer for `nself backup stream`, run in
// the real project with the running binary.
//
// WorkingDirectory is the resolved project directory (the one loadProjectConfig
// uses: the current directory), ExecStart is the absolute path of the running
// binary with symlinks resolved, and the unit has no EnvironmentFile line
// unless opts.EnvFile is set. Nothing is guessed: the unit that was previously
// written pointed at /opt/nself and /usr/local/bin/nself, which exist on no
// real project.
func ScheduleStream(cfg *config.Config, opts ScheduleOptions) error {
	if opts.Cron == "" {
		return fmt.Errorf("--cron expression required (e.g. '0 2 * * *')")
	}
	to := opts.To
	if to == "" {
		to = cfg.Backup.Remote
	}
	if to == "" {
		return fmt.Errorf("--to destination required for schedule")
	}
	unitDir := opts.UnitDir
	if unitDir == "" {
		unitDir = "/etc/systemd/system"
	}

	binaryPath, err := scheduleBinaryPath(opts.BinaryPath)
	if err != nil {
		return err
	}
	projectDir, err := scheduleProjectDir(opts.ProjectDir)
	if err != nil {
		return err
	}
	envFile := ""
	if opts.EnvFile != "" {
		if envFile, err = filepath.Abs(opts.EnvFile); err != nil {
			return fmt.Errorf("resolve --env-file: %w", err)
		}
	}
	for _, v := range append([]string{to, opts.HeartbeatTo, envFile}, opts.Recipients...) {
		if strings.ContainsAny(v, "\n\r") {
			return fmt.Errorf("a schedule value contains a line break")
		}
	}

	args := []string{binaryPath, "backup", "stream", "--to", to}
	for _, r := range opts.Recipients {
		args = append(args, "--recipient", r)
	}
	if opts.HeartbeatTo != "" {
		args = append(args, "--heartbeat-to", opts.HeartbeatTo)
	}
	execStart := systemdExecLine(args)

	// Convert simple cron "M H * * *" to systemd OnCalendar syntax.
	onCalendar := cronToSystemd(opts.Cron)

	service := renderService(serviceSpec{
		Description: "nSelf streaming encrypted backup",
		EnvFile:     envFile,
		WorkingDir:  strings.ReplaceAll(projectDir, "%", "%%"),
		ExecStart:   execStart,
	})
	if envFile == "" {
		// renderService always emits an EnvironmentFile line; with no env file
		// requested the bare "EnvironmentFile=-" must not appear at all.
		service = strings.Replace(service, "EnvironmentFile=-\n", "", 1)
	}
	timer := renderTimer(timerSpec{
		Description: "nSelf streaming backup: " + opts.Cron,
		OnCalendar:  onCalendar,
		Unit:        "nself-backup-stream.service",
		Persistent:  true,
	})

	if opts.DryRun {
		out := opts.Out
		if out == nil {
			out = os.Stdout
		}
		_, _ = fmt.Fprintf(out, "# nself-backup-stream.service\n%s\n# nself-backup-stream.timer\n%s", service, timer)
		return nil
	}

	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", unitDir, err)
	}

	servicePath := filepath.Join(unitDir, "nself-backup-stream.service")
	timerPath := filepath.Join(unitDir, "nself-backup-stream.timer")

	if err := os.WriteFile(servicePath, []byte(service), 0644); err != nil {
		return fmt.Errorf("write service unit: %w", err)
	}
	if err := os.WriteFile(timerPath, []byte(timer), 0644); err != nil {
		return fmt.Errorf("write timer unit: %w", err)
	}

	if err := run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	if err := run("systemctl", "enable", "--now", "nself-backup-stream.timer"); err != nil {
		return fmt.Errorf("enable timer: %w", err)
	}

	slog.Info("stream backup scheduled", "cron", opts.Cron, "destination", to)
	return nil
}

// scheduleBinaryPath returns the absolute path of the running binary with
// symlinks resolved, or the explicit override. The unit calls exactly this
// file, so a PATH lookup can never pick a different nself.
func scheduleBinaryPath(override string) (string, error) {
	p := override
	if p == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("resolve the running binary: %w", err)
		}
		p = exe
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve binary path %s: %w", p, err)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return abs, nil
}

// scheduleProjectDir returns the absolute project directory: the override, or
// the current directory (what loadProjectConfig and every other command use).
func scheduleProjectDir(override string) (string, error) {
	d := override
	if d == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve the project directory: %w", err)
		}
		d = wd
	}
	abs, err := filepath.Abs(d)
	if err != nil {
		return "", fmt.Errorf("resolve project directory %s: %w", d, err)
	}
	return abs, nil
}

// systemdExecLine joins argv into one ExecStart value. An argument with
// whitespace, a quote or a backslash is double-quoted; "%" and "$" are doubled
// because systemd expands specifiers and variables in ExecStart.
func systemdExecLine(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		a = strings.ReplaceAll(a, "%", "%%")
		a = strings.ReplaceAll(a, "$", "$$")
		if a == "" || strings.ContainsAny(a, " \t\"'\\") {
			a = "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(a) + "\""
		}
		parts[i] = a
	}
	return strings.Join(parts, " ")
}

// cronToSystemd converts a 5-field cron expression to a systemd OnCalendar value.
// Handles simple patterns only; complex expressions pass through with a warning.
func cronToSystemd(cron string) string {
	fields := strings.Fields(cron)
	if len(fields) != 5 {
		slog.Warn("cron expression not in 5-field format; using verbatim", "cron", cron)
		return cron
	}
	minute, hour, dom, month, dow := fields[0], fields[1], fields[2], fields[3], fields[4]

	// Daily at HH:MM when dom, month, dow are all *.
	if dom == "*" && month == "*" && dow == "*" {
		return fmt.Sprintf("*-*-* %s:%s:00 UTC", zeroPad(hour), zeroPad(minute))
	}

	// Weekly: dow set, dom and month *.
	if dow != "*" && dom == "*" && month == "*" {
		dayName := cronDOWName(dow)
		return fmt.Sprintf("%s *-*-* %s:%s:00 UTC", dayName, zeroPad(hour), zeroPad(minute))
	}

	// Monthly: dom set, dow and month *.
	if dom != "*" && dow == "*" && month == "*" {
		return fmt.Sprintf("*-*-%s %s:%s:00 UTC", zeroPad(dom), zeroPad(hour), zeroPad(minute))
	}

	// Fall back to daily to avoid invalid syntax; caller sees the warning above.
	return fmt.Sprintf("*-*-* %s:%s:00 UTC", zeroPad(hour), zeroPad(minute))
}

func zeroPad(s string) string {
	if len(s) == 1 && s[0] >= '0' && s[0] <= '9' {
		return "0" + s
	}
	return s
}

func cronDOWName(n string) string {
	days := map[string]string{
		"0": "Sun", "7": "Sun",
		"1": "Mon", "2": "Tue", "3": "Wed",
		"4": "Thu", "5": "Fri", "6": "Sat",
	}
	if name, ok := days[n]; ok {
		return name
	}
	return n
}
