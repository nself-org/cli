package commands

// Purpose: implements the runInit command handler for `nself init`, the interactive/flag-driven project scaffolding entrypoint.
// Inputs: cobra command flags (fast, force, dry-run, template selectors, domain overrides) and positional args from the invoking cobra.Command.
// Outputs: a scaffolded project directory (env files, docker assets, optional cloned/marketplace templates) or an error.
// Constraints: split out of init.go as a pure move (CLI-R12); no behavior change. Keep in sync with initCmd flag definitions in init.go.

import (
	"fmt"
	"os"
	"time"

	"github.com/nself-org/cli/internal/migration"
	"github.com/nself-org/cli/internal/scaffold"
	"github.com/nself-org/cli/internal/setup"
	"github.com/nself-org/cli/internal/telemetry"
	clonetemplate "github.com/nself-org/cli/internal/templates/clone"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func runInit(cmd *cobra.Command, args []string) error {
	fast, _ := cmd.Flags().GetBool("fast")
	interactive, _ := cmd.Flags().GetBool("interactive")
	nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
	template, _ := cmd.Flags().GetString("template")
	skipValidation, _ := cmd.Flags().GetBool("skip-validation")
	wizard, _ := cmd.Flags().GetBool("wizard")
	demo, _ := cmd.Flags().GetBool("demo")
	full, _ := cmd.Flags().GetBool("full")
	force, _ := cmd.Flags().GetBool("force")
	quiet, _ := cmd.Flags().GetBool("quiet")
	name, _ := cmd.Flags().GetString("name")
	domainFlag, _ := cmd.Flags().GetString("domain")
	noPgvector, _ := cmd.Flags().GetBool("no-pgvector")
	preset, _ := cmd.Flags().GetString("preset")
	listPresets, _ := cmd.Flags().GetBool("list-presets")
	csTemplate, _ := cmd.Flags().GetString("cs-template")

	// --list-presets/--cs-template/--preset validation + --name/--domain
	// sanitization, extracted to init_run_validate.go for 300-line compliance
	// (T-P6-E2-W1-S1-T3). done=true means runInit must return nil immediately
	// (the --list-presets case); err is returned as-is otherwise.
	var done bool
	var err error
	name, domainFlag, done, err = validateAndSanitizeInitFlags(csTemplate, preset, name, domainFlag, listPresets)
	if done {
		return nil
	}
	if err != nil {
		return err
	}

	if template != "" {
		if err := validateTemplate(template); err != nil {
			return err
		}

		// Clone templates are embedded in the binary and scaffold directly.
		if clonetemplate.IsCloneTemplate(template) {
			destDir := "."
			if len(args) > 0 {
				destDir = args[0]
			}
			noSeed, _ := cmd.Flags().GetBool("no-seed")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			return runInitCloneTemplate(template, destDir, noSeed, dryRun, force, quiet)
		}

		// Marketplace slug (not a built-in language template): fetch from registry.
		if !isBuiltinTemplate(template) {
			destDir := "."
			if len(args) > 0 {
				destDir = args[0]
			}
			return runInitMarketplaceTemplate(template, destDir, force, quiet)
		}
	}

	// v1 migration check: if .nself/ does not exist this may be a first run on an
	// existing v1 project. Scan for legacy artifacts and warn the user before init.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	if _, statErr := os.Stat(cwd + "/.nself"); os.IsNotExist(statErr) {
		if artifacts := migration.Detect(cwd); len(artifacts) > 0 {
			fmt.Fprintf(os.Stderr, "\n%s Legacy nSelf v1 artifacts detected:\n", ui.C(ui.Yellow, ui.IconWarning))
			for _, a := range artifacts {
				fmt.Fprintf(os.Stderr, "   %s %s  %s %s\n",
					ui.C(ui.Yellow, ui.IconBullet), a.Path,
					ui.C(ui.Dim, "\u2014"), ui.C(ui.Dim, a.Description+" ("+a.Action+")"),
				)
			}
			fmt.Fprintf(os.Stderr, "\nRun `nself migrate` to upgrade this project to v2.\n\n")
		}
	}

	if !quiet {
		ui.CommandHeader("nself init", "Initialize a new nSelf project")
	}

	// Tiny-VPS preflight: warn when available RAM is below the recommended
	// minimum (1 GB). Detection failure is non-fatal — we warn-and-continue.
	if ramMB, ramErr := getTotalMemoryMB(); ramErr == nil {
		const tinyThresholdMB = 1024
		const warnThresholdMB = 512
		if ramMB < warnThresholdMB {
			fmt.Fprintf(os.Stderr,
				"\n%s Detected %d MB RAM. nself default stack needs 1 GB+.\n"+
					"   For small VPS, run: nself init --profile=tiny\n"+
					"   The tiny profile starts Postgres + nginx only; Hasura/Auth are opt-in.\n"+
					"   See: https://github.com/nself-org/cli/wiki/install/tiny-vps\n\n",
				ui.C(ui.Yellow, ui.IconWarning), ramMB)
		} else if ramMB < tinyThresholdMB {
			fmt.Fprintf(os.Stderr,
				"\n%s Detected %d MB RAM. 1 GB+ recommended for the full stack.\n"+
					"   For small VPS, run: nself init --profile=tiny\n\n",
				ui.C(ui.Yellow, ui.IconWarning), ramMB)
		}
	}

	// Resolve domain: --domain takes precedence (validated); otherwise a TTY
	// run records the first preset, the former menu default, with no prompt.
	// Non-TTY / --non-interactive / --fast leave setup.go to apply its defaults.
	useDefault := !nonInteractive && !fast && term.IsTerminal(int(os.Stdin.Fd()))
	selectedDomain, selectedDomainComment, err := resolveInitDomain(domainFlag, useDefault)
	if err != nil {
		return err
	}

	opts := setup.Options{
		Fast:           fast,
		Interactive:    interactive,
		NonInteractive: nonInteractive,
		Template:       template,
		SkipValidation: skipValidation,
		Wizard:         wizard,
		Demo:           demo,
		Full:           full,
		Force:          force,
		Quiet:          quiet,
		Name:           name,
		Domain:         selectedDomain,
		DomainComment:  selectedDomainComment,
		NoPgvector:     noPgvector,
	}

	// Wizard progress: show step-by-step display when --wizard is active.
	var wizardSteps *ui.InitSteps
	if wizard && !quiet {
		wizardSteps = ui.NewInitSteps(false,
			"Validate inputs",
			"Generate secrets",
			"Write .env files",
			"Write .env.example",
			"Update .gitignore",
			"Create .nself/ directory",
		)
		fmt.Println()
		ui.Section("Wizard — initializing your nSelf project")
		fmt.Println()
		wizardSteps.Next() // step 1: inputs already validated above
		wizardSteps.Next() // step 2: generating secrets (happens inside Initialize)
	}

	// Telemetry: record start time for duration measurement.
	initStart := time.Now()

	result, err := setup.Initialize(opts)

	if wizardSteps != nil {
		// Advance remaining steps to reflect work completed inside Initialize.
		wizardSteps.Next() // step 3: .env files
		wizardSteps.Next() // step 4: .env.example
		wizardSteps.Next() // step 5: .gitignore
		wizardSteps.Next() // step 6: .nself/
		wizardSteps.Done()
		fmt.Println()
	}

	// Telemetry: emit init_complete event (opt-in only; silently no-ops when unset).
	if telemetry.IsOptedIn() {
		wizardMode := "default"
		switch {
		case fast:
			wizardMode = "fast"
		case wizard:
			wizardMode = "wizard"
		case demo:
			wizardMode = "demo"
		case nonInteractive:
			wizardMode = "non-interactive"
		}

		errCategory := ""
		if err != nil {
			errCategory = classifyInitError(err)
		}

		telemetry.Send("init_complete", map[string]any{
			"wizard_mode":  wizardMode,
			"duration_ms":  time.Since(initStart).Milliseconds(),
			"success":      err == nil,
			"err_category": errCategory,
		})
	}

	if err != nil {
		if !quiet {
			ui.Error(fmt.Sprintf("Init failed: %v", err))
		}
		return err
	}

	if quiet {
		return nil
	}

	// Summary.
	items := []string{
		fmt.Sprintf("Project:  %s", result.ProjectName),
		fmt.Sprintf("Domain:   %s", result.BaseDomain),
		fmt.Sprintf("Env:      %s", result.Env),
		fmt.Sprintf("Files:    %d created", len(result.FilesCreated)),
	}
	if name != "" {
		items = append(items, fmt.Sprintf("Name:     %s (from --name flag)", name))
	}
	if result.Demo {
		items = append(items, "Mode:     demo (all services enabled)")
	}
	ui.SummaryBox("Configuration Generated", items)

	// Next steps.
	ui.Section("Next steps")
	fmt.Printf("  1. %s  %s Verify environment (Docker, ports, config)\n",
		ui.C(ui.Bold, "nself doctor"), ui.C(ui.Dim, ui.IconArrow))
	fmt.Printf("  2. %s  %s Launch your backend stack\n",
		ui.C(ui.Bold, "nself start"), ui.C(ui.Dim, ui.IconArrow))
	fmt.Printf("  3. %s  %s Live health dashboard\n",
		ui.C(ui.Bold, "nself status"), ui.C(ui.Dim, ui.IconArrow))
	fmt.Printf("  4. %s  %s Browse available plugins\n",
		ui.C(ui.Bold, "nself plugin list"), ui.C(ui.Dim, ui.IconArrow))
	if result.Demo {
		fmt.Printf("  5. %s  %s Open the admin panel (demo mode)\n",
			ui.C(ui.Bold, "nself admin start"), ui.C(ui.Dim, ui.IconArrow))
	}
	fmt.Println()

	// Preset-specific next steps.
	if preset != "" {
		printPresetPostInit(preset)
	}

	// --cs-template: scaffold a custom service into the newly initialised project.
	if csTemplate != "" {
		svcName := "myservice"
		if name != "" {
			svcName = name
		}
		svcOpts := scaffold.Options{
			Name:       svcName,
			Lang:       csTemplate,
			ProjectDir: cwd,
		}
		if !quiet {
			ui.Section(fmt.Sprintf("Scaffolding custom service %q (%s)", svcName, csTemplate))
		}
		csResult, csErr := scaffold.Run(svcOpts)
		if csErr != nil {
			// Non-fatal: report but do not fail init.
			fmt.Fprintf(os.Stderr, "Warning: --cs-template scaffold failed: %v\n", csErr)
			fmt.Fprintln(os.Stderr, "Run 'nself service add <name> --lang "+csTemplate+"' to retry.")
		} else if !quiet {
			fmt.Printf("  Service %q scaffolded at %s\n", svcName, csResult.ServiceDir)
			fmt.Printf("  %s=%s written to %s\n", csResult.EnvKey, csResult.EnvValue, csResult.EnvFile)
			fmt.Printf("  Run 'nself build' then 'nself start' to launch.\n\n")
		}
	}

	return nil
}
