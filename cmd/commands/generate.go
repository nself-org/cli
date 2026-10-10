package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/generate"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// generateCmd is the top-level cobra command for schema codegen.
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate type-safe client SDK types from the live Hasura schema",
	Long: `Introspect the running nSelf Hasura instance and emit type-safe client
code for one or more target languages.

Supported languages: typescript, dart, swift, kotlin, python (default: all)

Codegen pipeline:
  1. Fetch live Hasura GraphQL schema via introspection (uses HASURA_GRAPHQL_ADMIN_SECRET)
  2. Parse schema into an internal IR (tables, enums, relationships)
  3. Merge .graphql operation files into the IR
  4. Render per-language output into --out directory
  5. Print diff summary: N types, M enums, K operations

Examples:
  nself generate                          # all languages, local env
  nself generate --lang typescript,dart   # subset of languages
  nself generate --env staging --lang typescript
  nself generate --watch`,
	RunE: dataEnv(runGenerate),
}

func init() {
	generateCmd.Flags().StringP("lang", "l", "all", "Comma-separated target languages: typescript,dart,swift,kotlin,python,all")
	generateCmd.Flags().StringP("out", "o", "./generated", "Output directory")
	generateCmd.Flags().StringP("env", "e", "local", "Target environment: local|staging|prod")
	generateCmd.Flags().Bool("watch", false, "Re-run on schema change (polls every 30s)")
	generateCmd.Flags().String("operations", "src/**/*.graphql", "Path to .graphql operation files")
	generateCmd.Flags().Bool("no-hooks", false, "Skip React hooks generation (TypeScript only)")
	generateCmd.Flags().Bool("pydantic", true, "Emit Pydantic v2 models (Python only)")

	RootCmd.AddCommand(generateCmd)
}

// generateLangs is the canonical ordered list of supported output languages.
var generateLangs = []string{"typescript", "dart", "swift", "kotlin", "python"}

// runGenerate is the cobra RunE handler for nself generate.
func runGenerate(cmd *cobra.Command, args []string) error {
	langFlag, _ := cmd.Flags().GetString("lang")
	outDir, _ := cmd.Flags().GetString("out")
	envFlag, _ := cmd.Flags().GetString("env")
	watch, _ := cmd.Flags().GetBool("watch")
	noHooks, _ := cmd.Flags().GetBool("no-hooks")

	langs, err := parseGenerateLangs(langFlag)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Require an nSelf project directory before attempting any external I/O
	// (Hasura introspection, schema fetch). Without this guard the command
	// proceeds to FetchSchema even from an empty directory, because config.Load
	// fills in defaults from os.Getenv rather than failing.
	if _, findErr := config.FindNSelfRoot(cwd); findErr != nil {
		return fmt.Errorf("no nself project found in %s — run 'nself init' first", cwd)
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	hasuraURL, adminSecret, err := resolveGenerateEndpoint(cfg, envFlag)
	if err != nil {
		return err
	}

	opts := generate.RenderOptions{
		Langs:    langs,
		OutDir:   outDir,
		NoHooks:  noHooks,
		Pydantic: true,
	}

	if watch {
		return runGenerateWatch(cmd.Context(), hasuraURL, adminSecret, opts, cfg.ProjectName)
	}

	return runGenerateOnce(cmd.Context(), hasuraURL, adminSecret, opts, cfg.ProjectName)
}

// runGenerateOnce performs a single codegen pass.
func runGenerateOnce(ctx context.Context, hasuraURL, adminSecret string, opts generate.RenderOptions, projectName string) error {
	ui.Info(fmt.Sprintf("Fetching schema from %s ...", hasuraURL))

	start := time.Now()
	schema, err := generate.FetchSchema(ctx, hasuraURL, adminSecret)
	if err != nil {
		return fmt.Errorf("fetch schema: %w", err)
	}

	ir := generate.BuildIR(schema, projectName)
	ui.Info(fmt.Sprintf("Parsed %d types, %d enums", len(ir.Tables), len(ir.Enums)))

	summaries, err := generate.Render(ir, opts)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}

	total := time.Since(start)
	for _, s := range summaries {
		ui.Success(fmt.Sprintf("[%s] %d files written (%.1fs)",
			s.Language, len(s.Files), s.Elapsed.Seconds()))
	}
	ui.Success(fmt.Sprintf("Done in %.1fs. Output: %s", total.Seconds(), opts.OutDir))
	return nil
}

// runGenerateWatch polls for schema changes every 30 seconds, re-generating on change.
func runGenerateWatch(ctx context.Context, hasuraURL, adminSecret string, opts generate.RenderOptions, projectName string) error {
	ui.Info("Watch mode active — polling every 30s. Press Ctrl+C to stop.")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Run immediately on start.
	if err := runGenerateOnce(ctx, hasuraURL, adminSecret, opts, projectName); err != nil {
		ui.Warn(fmt.Sprintf("Generate failed: %v", err))
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := runGenerateOnce(ctx, hasuraURL, adminSecret, opts, projectName); err != nil {
				ui.Warn(fmt.Sprintf("Generate failed: %v", err))
			}
		}
	}
}

// parseGenerateLangs validates and expands the --lang flag value.
func parseGenerateLangs(langFlag string) ([]string, error) {
	if langFlag == "all" {
		return generateLangs, nil
	}

	valid := map[string]bool{}
	for _, l := range generateLangs {
		valid[l] = true
	}

	raw := strings.Split(langFlag, ",")
	var langs []string
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if !valid[l] {
			return nil, fmt.Errorf("unsupported language %q; supported: %s",
				l, strings.Join(generateLangs, ", "))
		}
		langs = append(langs, l)
	}

	if len(langs) == 0 {
		return nil, fmt.Errorf("no languages specified")
	}
	return langs, nil
}

// resolveGenerateEndpoint returns the Hasura URL and admin secret for the
// requested environment. Env vars take precedence over config file values.
func resolveGenerateEndpoint(cfg *config.Config, env string) (string, string, error) {
	switch env {
	case "local", "":
		port := cfg.Hasura.Port
		if port == 0 {
			port = 8080
		}
		url := fmt.Sprintf("http://localhost:%d", port)
		secret := cfg.Hasura.AdminSecret
		if secret == "" {
			secret = os.Getenv("HASURA_GRAPHQL_ADMIN_SECRET")
		}
		if secret == "" {
			return "", "", fmt.Errorf("HASURA_GRAPHQL_ADMIN_SECRET is not set; run 'nself start' first")
		}
		return url, secret, nil

	case "staging":
		url := os.Getenv("NSELF_HASURA_STAGING_URL")
		if url == "" {
			url = fmt.Sprintf("https://api.%s", cfg.BaseDomain)
		}
		secret := os.Getenv("NSELF_HASURA_STAGING_ADMIN_SECRET")
		if secret == "" {
			secret = cfg.Hasura.AdminSecret
		}
		if secret == "" {
			return "", "", fmt.Errorf("NSELF_HASURA_STAGING_ADMIN_SECRET is not set")
		}
		return url, secret, nil

	case "prod":
		url := os.Getenv("NSELF_HASURA_PROD_URL")
		if url == "" {
			return "", "", fmt.Errorf("NSELF_HASURA_PROD_URL is not set; required for --env prod")
		}
		secret := os.Getenv("NSELF_HASURA_PROD_ADMIN_SECRET")
		if secret == "" {
			return "", "", fmt.Errorf("NSELF_HASURA_PROD_ADMIN_SECRET is not set; required for --env prod")
		}
		return url, secret, nil

	default:
		return "", "", fmt.Errorf("unknown env %q; expected: local, staging, prod", env)
	}
}
