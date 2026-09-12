package build

// orchestrator_build_compose.go — Build() Steps 8-9.8: generate
// docker-compose.yml, merge the Ollama sidecar, template secrets out of the
// YAML, write the compose file + plugin compose manifest, warn on plugin
// Dockerfiles missing HEALTHCHECK, wire ɳSentry/Loki monitoring configs, and
// refresh hasura/config.yaml. Split from orchestrator.go (T-P6-E2-W1-S1-T3).
// Inputs:  st.workdir, st.opts, st.cfg, st.pluginDir (set by the previous
//          phase).
// Outputs: sets st.composePath, st.secretMap, st.pluginComposeFiles;
//          increments st.filesGenerated. Returns error on any generation
//          failure.
// Constraints: pure move, same checks/output/errors/order, no behavior
//              change.

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/compose/monitoring"
)

// generateCompose runs Steps 8-9.8 of Build().
func (st *buildState) generateCompose() error {
	// ── Step 8: Generate docker-compose.yml ─────────────────────────
	// Profile selection: empty or "app" → full service set (no regression).
	// "ops" → observability + CI + functions + registry, no app services.
	profile := st.opts.Profile
	if profile == "" {
		profile = compose.ProfileApp
	}
	composeGen := compose.NewGeneratorWithProfile(st.cfg, profile).WithWorkDir(st.workdir)
	composeYAML, err := composeGen.Generate()
	if err != nil {
		return fmt.Errorf("generating docker-compose.yml: %w", err)
	}

	// ── Step 8.1: Warn if this regen would swap a running postgres image ──
	// (cli#384 — advisory only, never blocks the build; see
	// PostgresImageChangeWarning for the data-loss scenario this guards.)
	postgresContainer := fmt.Sprintf("%s_postgres", st.cfg.ProjectName)
	generatedPostgresImage := compose.ResolveImage("postgres", compose.ResolvePostgresImage(st.cfg.Postgres))
	if warning := PostgresImageChangeWarning(postgresContainer, generatedPostgresImage); warning != "" {
		slog.Warn(warning)
	}

	// ── Step 8.5: Merge Ollama sidecar when AI_OLLAMA_ENABLED=true ──
	ollamaEnv := collectOllamaEnv()
	mergedYAML, ollamaInjected, err := MergeOllamaSidecar(composeYAML, ollamaEnv)
	if err != nil {
		return fmt.Errorf("merging ollama sidecar: %w", err)
	}
	if ollamaInjected {
		composeYAML = mergedYAML
		if st.opts.Verbose {
			slog.Info("Ollama sidecar injected", "file", "docker-compose.yml", "trigger", "AI_OLLAMA_ENABLED=true")
		}
	}

	// ── Step 8.7: Template secrets out of the generated YAML ────────
	// Literal passwords/keys become ${VAR} references resolved at
	// container-start time from .nself/compose.env (written in Step 10 and
	// passed via --env-file by start/stop/restart). Secrets never land in
	// the generated docker-compose.yml (ASI generated-file-secret rule).
	st.secretMap = SecretEnvMap(st.cfg)
	composeYAML = TemplateSecrets(composeYAML, st.secretMap)
	for _, leak := range LiteralSecretLeaks(composeYAML, st.secretMap) {
		slog.Warn("secret value still appears literally in docker-compose.yml — do not commit this file",
			"var", leak)
	}

	// ── Step 9: Write docker-compose.yml with 0600 permissions ──────
	// Prepend the GENERATED marker so pre-commit hooks, auditors, and humans
	// can unambiguously detect a hand-edited compose file (S32-T12).
	composeYAML = prependGeneratedHeader(composeYAML)
	st.composePath = filepath.Join(st.workdir, "docker-compose.yml")
	if err := os.WriteFile(st.composePath, composeYAML, 0600); err != nil {
		return fmt.Errorf("writing docker-compose.yml: %w", err)
	}
	st.filesGenerated++

	// ── Step 9.5: Discover plugin compose files ────────────────────
	pluginComposeFiles, err := DiscoverPluginComposeFiles(st.workdir, st.pluginDir)
	if err != nil {
		return fmt.Errorf("discovering plugin compose files: %w", err)
	}
	st.pluginComposeFiles = pluginComposeFiles

	// Write the compose manifest so start/stop/restart know which files
	// to pass as -f flags to docker compose.
	if err := WriteComposeManifest(st.workdir, st.composePath, st.pluginComposeFiles); err != nil {
		return fmt.Errorf("writing compose manifest: %w", err)
	}
	st.filesGenerated++

	if st.opts.Verbose && len(st.pluginComposeFiles) > 0 {
		slog.Info("discovered plugin compose files", "count", len(st.pluginComposeFiles), "files", st.pluginComposeFiles)
	}

	// ── Step 9.6: Verify Go plugin Dockerfiles have HEALTHCHECK ─────
	for _, w := range CheckGoPluginDockerfiles(st.pluginDir) {
		slog.Warn(w)
	}

	// ── Step 9.7: Wire ɳSentry scrape targets + Loki configs ────────
	// When the monitoring bundle is enabled (MONITORING_ENABLED=true), emit
	// monitoring/prometheus.yml with builtin targets + any installed ɳSentry
	// plugin targets (S12.T01) and monitoring/loki.yml + promtail.yml (S9.T08).
	//
	// Both writes are idempotent: identical inputs produce byte-identical files.
	// When monitoring is disabled, this step is a no-op so projects that opt out
	// don't acquire stray monitoring/*.yml files.
	if st.cfg.Monitoring.Enabled {
		// Prometheus: build PrometheusConfig with builtin targets, append any
		// installed ɳSentry plugin targets, render and write to disk.
		promCfg := monitoring.Defaults()
		nsentryAdded := AppendNSentryTargets(promCfg, st.pluginDir)
		promYAML, err := monitoring.RenderPrometheusYAML(promCfg)
		if err != nil {
			return fmt.Errorf("rendering monitoring/prometheus.yml: %w", err)
		}
		monDir := filepath.Join(st.workdir, "monitoring")
		if err := os.MkdirAll(monDir, 0o755); err != nil {
			return fmt.Errorf("creating monitoring dir: %w", err)
		}
		promPath := filepath.Join(monDir, "prometheus.yml")
		if err := atomicWrite(promPath, promYAML, 0o644); err != nil {
			return fmt.Errorf("writing monitoring/prometheus.yml: %w", err)
		}
		st.filesGenerated++
		if st.opts.Verbose {
			if nsentryAdded > 0 {
				slog.Info("generated monitoring/prometheus.yml", "nsentry_targets", nsentryAdded)
			} else {
				slog.Info("generated monitoring/prometheus.yml", "nsentry_targets", 0)
			}
		}

		// Loki: render loki.yml + promtail.yml with project-name + retention
		// taken from the monitoring config (env-cascade applied).
		lokiOpts := LokiBuildOptions{
			ProjectName: st.cfg.ProjectName,
		}
		if r := strings.TrimSpace(os.Getenv("LOKI_RETENTION_PERIOD")); r != "" {
			lokiOpts.RetentionPeriod = r
		}
		nLoki, err := WriteLokiConfigs(st.workdir, lokiOpts)
		if err != nil {
			return fmt.Errorf("writing Loki configs: %w", err)
		}
		st.filesGenerated += nLoki
		if st.opts.Verbose {
			slog.Info("generated monitoring configs", "files", nLoki)
		}
	}

	// ── Step 9.8: Refresh hasura/config.yaml from the resolved cascade ──
	// Only touches projects that already use the Hasura CLI project layout
	// (a hasura/ directory present) so repos that don't run hasura-cli by
	// hand are unaffected (T-gap-10, backward compatible).
	if info, statErr := os.Stat(filepath.Join(st.workdir, "hasura")); statErr == nil && info.IsDir() {
		n, err := WriteHasuraCLIConfig(st.workdir, st.cfg)
		if err != nil {
			return fmt.Errorf("writing hasura/config.yaml: %w", err)
		}
		st.filesGenerated += n
		if st.opts.Verbose && n > 0 {
			slog.Info("generated hasura/config.yaml", "endpoint", fmt.Sprintf("http://localhost:%d", st.cfg.Hasura.Port))
		}
	}

	return nil
}
