package commands

// Purpose: `nself doctor report` writes a redacted support bundle (EPIC D21).
// Inputs: --output <file> (default nself-report-<UTC>.tar.gz in the project
//         dir), --log-lines N (default 200), --json.
// Outputs: a 0600 tar.gz (versions, env key names, generated compose and nginx
//          confs, doctor run, per-service log tails, manifest) and its path on
//          stdout; with --json one v1 envelope {path, files, bytes, redactions}.
// Constraints: every file passes the scrubber in internal/doctor/report before
//          it is archived; env values (cascade, process environment, compose
//          fragments, CS_N_ENV_FILE files) are scrub inputs only; the file is
//          never overwritten; nothing is uploaded. A missing stack is not an
//          error: the archive then holds an empty logs/ directory.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/doctor"
	"github.com/nself-org/cli/internal/doctor/report"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	reportDefaultLogLines = 200
	reportMaxLogLines     = 10000
)

var doctorReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Write a redacted support bundle (tar.gz) to share in an issue",
	Long: `Collect versions, env key names (never values), the generated compose and
nginx configs, a doctor run and recent service logs into one 0600 tar.gz.
Every file is scrubbed first: known secret values, PEM keys, URL credentials
and token patterns are replaced by [REDACTED]. The scrubber removes known values
and known patterns only, so review the archive before you share it. Nothing is
uploaded.`,
	Args: cobra.NoArgs,
	RunE: runDoctorReport,
}

func init() {
	doctorReportCmd.Flags().String("output", "", "Archive path (default nself-report-<UTC timestamp>.tar.gz in the project directory)")
	doctorReportCmd.Flags().Int("log-lines", reportDefaultLogLines, "Log lines to keep per service")
	doctorCmd.AddCommand(doctorReportCmd)
}

// DoctorReport is the envelope data of `doctor report --json`.
type DoctorReport struct {
	Path       string `json:"path"`       // absolute path of the archive
	Files      int    `json:"files"`      // entries in the archive, manifest.json included
	Bytes      int64  `json:"bytes"`      // archive size
	Redactions int    `json:"redactions"` // total replacements made across all files
}

// doctorReportSources is the seam tests replace; production wires the real
// docker funnel and the doctor child run.
var doctorReportSources = report.RealSources

func runDoctorReport(cmd *cobra.Command, _ []string) error {
	jsonOn, err := jsonModeOf(cmd)
	if err != nil {
		return err
	}
	lines, _ := cmd.Flags().GetInt("log-lines")
	if lines < 1 || lines > reportMaxLogLines {
		return fmt.Errorf("--log-lines must be between 1 and %d", reportMaxLogLines)
	}
	projectDir, err := resolveProjectDir()
	if err != nil {
		return err
	}
	ds, setupNote := reportDockerSetup(projectDir)
	src := doctorReportSources(ds)
	if src.Now == nil {
		src.Now = time.Now
	}
	opts := report.Options{ProjectDir: projectDir, LogLines: lines, ComposeFragments: ds.ComposeFiles}
	if setupNote != "" {
		opts.Notes = append(opts.Notes, setupNote)
	}
	var note string
	if opts.NginxDir, note = reportNginxDir(projectDir); note != "" {
		opts.Notes = append(opts.Notes, note)
	}
	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	// Ctrl-C (or SIGTERM) cancels the collection and returns, so nothing is
	// claimed or left behind in the output directory (N2).
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	bundle, err := report.Collect(ctx, opts, src)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return errors.New("interrupted: no report was written")
	}
	out, _ := cmd.Flags().GetString("output")
	if out == "" {
		out = filepath.Join(projectDir, report.DefaultName(src.Now()))
	}
	if out, err = filepath.Abs(out); err != nil {
		return err
	}
	size, err := report.WriteTarball(bundle, out, src.Now())
	if err != nil {
		return err
	}
	if jsonOn {
		return output.EmitData(pilotWriter(), "doctor report", DoctorReport{Path: out, Files: len(bundle.Files), Bytes: size, Redactions: bundle.Total})
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", out)
	return err
}

// reportNginxDir resolves the nginx directory the stack serves (nginxtopo: the
// project's own, or the fronting stack's when NGINX_FRONTED_BY is set). An
// unresolvable layout returns "" and a note; the report goes on without nginx.
func reportNginxDir(projectDir string) (dir, note string) {
	fronted := ""
	if cfg, err := doctor.LoadConfigIsolated(projectDir); err == nil {
		fronted = cfg.Nginx.FrontedBy
	}
	dir, err := nginxtopo.ServedNginxDir(projectDir, fronted)
	if err != nil {
		return "", "nginx configs skipped: " + err.Error()
	}
	return dir, ""
}

// reportDockerSetup resolves the compose files and env files of the project.
// An unreadable compose manifest returns a note: the fragments it lists are
// scrub inputs, so the report must say they were skipped.
func reportDockerSetup(projectDir string) (ds report.DockerSetup, note string) {
	ds = report.DockerSetup{ProjectDir: projectDir}
	files, err := build.ReadComposeManifest(projectDir)
	if err != nil {
		files = nil
		note = "compose manifest unreadable: fragment values were not scrubbed as inputs"
	}
	for _, f := range files {
		if !filepath.IsAbs(f) {
			f = filepath.Join(projectDir, f)
		}
		if st, serr := os.Stat(f); serr == nil && st.Mode().IsRegular() {
			ds.ComposeFiles = append(ds.ComposeFiles, f)
		}
	}
	if len(ds.ComposeFiles) == 0 {
		ds.ComposeFiles = []string{filepath.Join(projectDir, "docker-compose.yml")}
	}
	env, _ := config.ResolveEnv(projectDir)
	for _, f := range config.EnvCascade(projectDir, env, config.LegacyOrderActive()) {
		if f.Exists {
			ds.EnvFiles = append(ds.EnvFiles, f.Path)
		}
	}
	return ds, note
}
