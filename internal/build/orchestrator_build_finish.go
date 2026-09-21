package build

// orchestrator_build_finish.go — Build() Steps 10-12: write .env.computed,
// write .nself/compose.env, save the build version, generate the OpenAPI
// spec + Scalar HTML page, run post-build validation, and assemble the
// final BuildResult. Split from orchestrator.go (T-P6-E2-W1-S1-T3).
// Inputs:  st (fully populated by the previous three phases).
// Outputs: *BuildResult on success, error on any generation or post-build
//          validation failure.
// Constraints: pure move, same checks/output/errors/order, no behavior
//              change.

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/apidocs"
	"github.com/nself-org/cli/internal/version"
)

// writeFinalArtifacts runs Steps 10-12 of Build().
func (st *buildState) writeFinalArtifacts() (*BuildResult, error) {
	// ── Step 10: Write .env.computed ────────────────────────────────
	pluginEnvVars := ComputePluginEnvVars(st.workdir, st.pluginDir, st.cfg)
	computedPath := filepath.Join(st.workdir, ".env.computed")
	computedContent := buildEnvComputed(st.cfg, pluginEnvVars)
	if err := os.WriteFile(computedPath, []byte(computedContent), 0600); err != nil {
		return nil, fmt.Errorf("writing .env.computed: %w", err)
	}
	st.filesGenerated++

	// ── Step 10.1: Write .nself/compose.env (0600) ──────────────────
	// Resolves every ${VAR} reference the secret-templating pass (Step 8.7)
	// emitted, plus plugin fragment vars (DOCKER_NETWORK, NSELF_PLUGIN_DIR,
	// PLUGIN_*_INTERNAL_URL). Passed to docker compose via --env-file.
	if err := WriteComposeEnv(st.workdir, st.cfg, st.secretMap, pluginEnvVars); err != nil {
		return nil, fmt.Errorf("writing %s: %w", composeEnvFile, err)
	}
	st.filesGenerated++

	// ── Step 11: Save build version to .nself/build-version ─────────
	versionPath := filepath.Join(st.workdir, buildVersionFile)
	if err := os.WriteFile(versionPath, []byte(version.GetVersion()), 0644); err != nil {
		return nil, fmt.Errorf("writing build version: %w", err)
	}
	st.filesGenerated++

	// Record the profile too, so the next build notices a switch.
	if err := RecordProfile(st.workdir, string(st.opts.Profile)); err != nil {
		return nil, fmt.Errorf("writing build profile: %w", err)
	}
	st.filesGenerated++

	// ── Step 11.6: Generate OpenAPI 3.1 spec + Scalar HTML page ─────
	// Only runs when api_docs.enabled is true (default). Writes two files:
	//   .nself/dist/openapi.json   — served at /api-docs by nginx
	//   .nself/dist/scalar.html    — served at /docs (or custom path)
	// Also writes nginx/conf.d/api-docs.conf with the location blocks.
	apiDocsCfg := apidocs.ApiDocsConfig{
		Enabled:         st.cfg.ApiDocs.Enabled,
		Path:            st.cfg.ApiDocs.Path,
		Title:           st.cfg.ApiDocs.Title,
		Theme:           st.cfg.ApiDocs.Theme,
		AuthEnvVar:      st.cfg.ApiDocs.AuthEnvVar,
		HideEndpoints:   st.cfg.ApiDocs.HideEndpoints,
		GraphQLEnabled:  st.cfg.ApiDocs.GraphQLEnabled,
		GraphQLEndpoint: st.cfg.ApiDocs.GraphQLEndpoint,
	}
	// Default-fill when the config section was left empty.
	if !apiDocsCfg.Enabled && st.cfg.ApiDocs.Path == "" {
		apiDocsCfg = apidocs.DefaultApiDocsConfig()
	}
	if apiDocsCfg.Enabled {
		pluginDir := DefaultPluginDir()
		pluginRoutes, err := apidocs.CollectPluginRoutes(pluginDir)
		if err != nil {
			slog.Warn("collecting plugin API routes", "err", err)
		}
		if _, err := apidocs.Generate(st.workdir, st.cfg.ProjectName, st.cfg.BaseDomain, apiDocsCfg, pluginRoutes); err != nil {
			return nil, fmt.Errorf("generating api docs: %w", err)
		}
		st.filesGenerated += 2 // openapi.json + scalar.html

		// Write the nginx site config (full server block, served on docs.<base>).
		apiDocsNginxConf := apidocs.NginxConf(apiDocsCfg.Path, st.cfg.BaseDomain)
		apiDocsConfPath := filepath.Join(st.workdir, "nginx", "sites", "api-docs.conf")
		if err := os.WriteFile(apiDocsConfPath, []byte(apiDocsNginxConf), 0644); err != nil {
			return nil, fmt.Errorf("writing api-docs nginx conf: %w", err)
		}
		// Best-effort cleanup of the legacy bare-location file, if present from a
		// prior build with the broken layout.
		_ = os.Remove(filepath.Join(st.workdir, "nginx", "conf.d", "api-docs.conf"))
		st.filesGenerated++
	}

	// ── Step 11.5: Post-build validation ────────────────────────────
	nginxSitesDir := filepath.Join(st.workdir, "nginx", "sites")
	pvResult := PostValidate(st.composePath, nginxSitesDir)

	// Print warnings — they do not fail the build.
	for _, w := range pvResult.Warnings {
		slog.Warn(w)
	}

	// Any errors from post-validation fail the build.
	if len(pvResult.Errors) > 0 {
		msg := "post-build validation failed:\n"
		for _, e := range pvResult.Errors {
			msg += "  - " + e + "\n"
		}
		return nil, fmt.Errorf("%s", msg)
	}

	// ── Step 12: Return BuildResult with summary ────────────────────
	return &BuildResult{
		ProjectName:        st.cfg.ProjectName,
		ComposeFile:        st.composePath,
		NginxConfig:        filepath.Join(st.workdir, "nginx", "nginx.conf"),
		SSLCerts:           st.sslResult.Count,
		Duration:           time.Since(st.start),
		FilesGenerated:     st.filesGenerated,
		PluginComposeFiles: st.pluginComposeFiles,
		MissingPlugins:     st.missingPlugins,
		CAInstalled:        st.sslResult.CAInstalled,
		CAManualCmd:        st.sslResult.CAManualCmd,
		HostsAdded:         st.sslResult.HostsAdded,
		HostsManualNote:    st.sslResult.HostsManualNote,
	}, nil
}
