package commands

// db_image.go — `nself db image switch --to pgvector` (EPIC P7-ADOPT D6, P7-ADOPT-22).
//
// Purpose: move an existing non-pgvector Postgres cluster to the pgvector image
// of the same major version. The data is dumped, restored into a NEW volume
// inside a one-shot server and verified before env, compose and lock change; the
// old volume is kept. --plan previews, --resume continues, --rollback goes back.
// Inputs: --to pgvector, --plan, --yes, --resume, --rollback, --json.
// Outputs: a progress log on stderr and a summary, or the v1 envelope.
// Constraints: destructive side effect (registry): a non-interactive run needs
// --yes; the engine is internal/postgres/imageswitch and this file only wires
// flags, config, compose and reconcile to it.

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/postgres/imageswitch"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

var dbImageCmd = &cobra.Command{Use: "image", Short: "Manage the Postgres image of a project", Args: cobra.NoArgs}

var dbImageSwitchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Move the database to the pgvector Postgres image (the old volume is kept)",
	Long: `Move an existing Postgres cluster to the pgvector image of the same major version.

The databases are dumped from a read-only cluster, restored into a NEW volume inside
a one-shot server, and checked (rows, schema, sequences, roles, grants) before the
project's env, compose file and lock change. The old volume is never touched; the
services are stopped for the length of the move (--plan prints an estimate).

  nself db image switch --to pgvector --plan    preview, change nothing
  nself db image switch --to pgvector --yes     run it
  nself db image switch --resume                finish an interrupted switch
  nself db image switch --rollback --yes        go back to the old volume`,
	Args: cobra.NoArgs,
	RunE: runDBImageSwitch,
}

func init() {
	f := dbImageSwitchCmd.Flags()
	f.String("to", "", "Target image: pgvector")
	f.Bool("plan", false, "Show the steps, the downtime estimate and the compose and lock change; change nothing")
	f.Bool("yes", false, "Confirm without a prompt (required when not interactive)")
	f.Bool("resume", false, "Continue an incomplete switch")
	f.Bool("rollback", false, "Go back to the old volume (works from any phase and after success)")
	f.Bool("json", false, "Emit a v1 JSON envelope")
	dbImageCmd.AddCommand(dbImageSwitchCmd)
	dbCmd.AddCommand(dbImageCmd)
}

func runDBImageSwitch(cmd *cobra.Command, _ []string) error {
	fl := cmd.Flags()
	plan, _ := fl.GetBool("plan")
	resume, _ := fl.GetBool("resume")
	rollback, _ := fl.GetBool("rollback")
	yes, _ := fl.GetBool("yes")
	asJSON, _ := fl.GetBool("json")
	to, _ := fl.GetString("to")
	if rollback && (plan || resume) {
		return errs.New("E401", "--rollback cannot be combined with --plan or --resume")
	}
	if !rollback && !resume && to != "pgvector" {
		return errs.New("E401", "--to pgvector is required: pgvector is the only target image")
	}
	dir, err := resolveProjectDir()
	if err != nil {
		return err
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("image switch: project config could not be resolved")
	}
	ctx, stop := signal.NotifyContext(cmdContext(cmd), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	sw := imageswitch.New(imageSwitchConfig(dir, cfg), imageSwitchDepsFor(cmd, dir, asJSON))
	var rep *imageswitch.Report
	switch {
	case plan:
		rep, err = sw.Plan(ctx)
		if err != nil && rep != nil && !asJSON {
			_ = renderImageSwitch(cmd, rep)
		}
	case rollback:
		rep, err = imageSwitchRollback(ctx, cmd, sw, yes)
	default:
		if err = confirmImageSwitch(cmd, yes, resume); err == nil {
			rep, err = sw.Run(ctx, imageswitch.RunOptions{Resume: resume})
		}
	}
	if err != nil {
		return err
	}
	if asJSON {
		return output.EmitData(output.Default(), "db image switch", rep)
	}
	return renderImageSwitch(cmd, rep)
}

// cmdContext is the command context, never nil.
func cmdContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// imageSwitchRollback runs a rollback; after success it asks for confirmation
// when interactive and --yes was not given.
func imageSwitchRollback(ctx context.Context, cmd *cobra.Command, sw *imageswitch.Switch, yes bool) (*imageswitch.Report, error) {
	rep, err := sw.Rollback(ctx, yes)
	if d := errs.Describe(err); d != nil && d.Code == "E403" {
		if ask := interactiveConfirm(cmd); ask != nil && ask("Writes since the switch stay in the new volume only. Go back to the old volume?") {
			return sw.Rollback(ctx, true)
		}
	}
	return rep, err
}

// confirmImageSwitch asks before a run that stops the stack. --yes skips it; a
// non-interactive run without --yes is refused (E403).
func confirmImageSwitch(cmd *cobra.Command, yes, resume bool) error {
	if yes || resume {
		return nil
	}
	if ask := interactiveConfirm(cmd); ask != nil {
		if ask("Stop the stack, dump the databases and move them to a pgvector volume (the old volume is kept; --plan shows the downtime estimate)?") {
			return nil
		}
		return errs.New("E403", "image switch cancelled; nothing was changed")
	}
	return errs.New("E403", "the image switch stops the stack and needs confirmation; nothing was changed").
		WithWhy("the run is not interactive").
		WithFix("preview it with --plan, then run it with --yes")
}

// renderImageSwitch prints the human summary of a report.
func renderImageSwitch(cmd *cobra.Command, r *imageswitch.Report) error {
	out := cmd.OutOrStdout()
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(out, format+"\n", a...) }
	switch r.Status {
	case "already-pgvector":
		p("Postgres is already on pgvector (%s); nothing changed.", r.OldImage)
		return nil
	case "rolled-back":
		p("Rolled back: the project runs on the old volume %s again.", r.OldVolume)
	case "switched":
		p("Switched to %s on the new volume %s.", r.NewImage, r.NewVolume)
		p("Verified %d databases, %d tables, %d sequences; downtime %ds.", r.Databases, r.Tables, r.Sequences, r.DowntimeSeconds)
	default:
		p("Plan: move %s (%s) to %s on a new volume %s.", r.OldVolume, r.OldImage, r.NewImage, r.NewVolume)
		p("Data %d bytes, %d databases; estimated downtime %ds; dumps go to %s (mode 0700).", r.DataBytes, r.Databases, r.DowntimeSeconds, r.Stage)
		for _, s := range r.Steps {
			p("  - %s", s)
		}
	}
	for _, c := range r.Preflight {
		mark := "ok"
		if !c.OK {
			mark = "FAILED"
		}
		p("  [%s] %s: %s", mark, c.Name, c.Detail)
	}
	if bp, ok := r.Build.(*reconcile.Plan); ok && bp != nil {
		p("Compose and lock change:")
		if err := reconcile.RenderHuman(out, *bp); err != nil {
			return err
		}
	}
	for _, t := range r.Tolerated {
		p("  tolerated: %s", t)
	}
	for _, w := range r.Warnings {
		p("warning: %s", w)
	}
	for _, n := range r.NextSteps {
		p("next: %s", n)
	}
	return nil
}
