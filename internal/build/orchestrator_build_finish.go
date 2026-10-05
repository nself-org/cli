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
	"cmp"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/apidocs"
	"github.com/nself-org/cli/internal/version"
)

// WriteAPIDocsSiteConf writes the api-docs server block (docs.<base>) to
// api-docs.conf in the SERVED nginx sites directory and returns its path.
//
// Inputs: the project workdir, NGINX_FRONTED_BY (empty for a project that runs
// its own nginx) and the rendered conf. Outputs: the written path. On a fronted
// project the fronting stack's nginx/sites is used (D-0045, P7-LIVE-02) — the
// tree its nginx reads; an unconfirmed layout is the same refusal build gives
// for every other site conf. Non-fronted output is <workdir>/nginx/sites as
// before.
func WriteAPIDocsSiteConf(workdir, frontedBy string, conf []byte) (string, error) {
	return writeAPIDocsSiteConfVia(newDiskSink(workdir), workdir, frontedBy, conf)
}

// writeAPIDocsSiteConfVia is WriteAPIDocsSiteConf writing through sink.
func writeAPIDocsSiteConfVia(sink Sink, workdir, frontedBy string, conf []byte) (string, error) {
	sitesDir, err := resolveNginxSitesDir(workdir, frontedBy)
	if err != nil {
		return "", fmt.Errorf("resolving nginx sites dir for api-docs: %w", err)
	}
	if err := sink.MkdirAll(sitesDir, 0755); err != nil {
		return "", fmt.Errorf("creating %s: %w", sitesDir, err)
	}
	path := filepath.Join(sitesDir, "api-docs.conf")
	if err := sink.WriteFile(path, conf, 0644); err != nil {
		return "", fmt.Errorf("writing api-docs nginx conf: %w", err)
	}
	return path, nil
}

// writeFinalArtifacts runs Steps 10-12 of Build().
func (st *buildState) writeFinalArtifacts() (*BuildResult, error) {
	st.ensureSeam()
	// ── Step 10: Write .env.computed ────────────────────────────────
	pluginEnvVars := ComputePluginEnvVars(st.workdir, st.pluginDir, st.cfg)
	computedPath := filepath.Join(st.workdir, ".env.computed")
	computedContent := buildEnvComputed(st.cfg, pluginEnvVars)
	if err := st.sink.WriteFile(computedPath, []byte(computedContent), 0600); err != nil {
		return nil, fmt.Errorf("writing .env.computed: %w", err)
	}
	st.filesGenerated++

	// ── Step 10.1: Write .nself/compose.env (0600) ──────────────────
	// Resolves every ${VAR} reference the secret-templating pass (Step 8.7)
	// emitted, plus plugin fragment vars (DOCKER_NETWORK, NSELF_PLUGIN_DIR,
	// PLUGIN_*_INTERNAL_URL). Passed to docker compose via --env-file.
	if err := writeComposeEnvVia(st.sink, st.workdir, st.cfg, st.secretMap, pluginEnvVars); err != nil {
		return nil, fmt.Errorf("writing %s: %w", composeEnvFile, err)
	}
	st.filesGenerated++

	// ── Step 11: Save build version to .nself/build-version ─────────
	versionPath := filepath.Join(st.workdir, buildVersionFile)
	if err := st.sink.WriteFile(versionPath, []byte(version.GetVersion()), 0644); err != nil {
		return nil, fmt.Errorf("writing build version: %w", err)
	}
	st.filesGenerated++

	// Record the profile too, so the next build notices a switch.
	// Same bytes and mode as RecordProfile (cache.go), written through the sink.
	profile := cmp.Or(string(st.opts.Profile), "app")
	if err := st.sink.WriteFile(filepath.Join(st.workdir, buildProfileFile), []byte(profile), 0644); err != nil {
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
		if err := st.writeAPIDocs(apiDocsCfg, pluginRoutes); err != nil {
			return nil, err
		}
		st.filesGenerated += 2 // openapi.json + scalar.html

		// Write the nginx site config (full server block, served on docs.<base>).
		apiDocsNginxConf := apidocs.NginxConf(apiDocsCfg.Path, st.cfg.BaseDomain)
		if _, err := writeAPIDocsSiteConfVia(st.sink, st.workdir, st.cfg.Nginx.FrontedBy, []byte(apiDocsNginxConf)); err != nil {
			return nil, err
		}
		// Best-effort cleanup of the legacy bare-location file, if present from a
		// prior build with the broken layout.
		_ = st.sink.Remove(filepath.Join(st.workdir, "nginx", "conf.d", "api-docs.conf"))
		st.filesGenerated++
	}

	// ── Step 11.5: Post-build validation ────────────────────────────
	nginxSitesDir := filepath.Join(st.workdir, "nginx", "sites")
	var pvResult PostValidateResult
	if !st.planning() {
		// PostValidate reads the written tree from disk (and may run `nginx -t`),
		// so plan mode skips it; the plan's artifacts are validated by whoever
		// applies them (P7-LIVE-03).
		pvResult = PostValidate(st.composePath, nginxSitesDir)
	}

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

// writeAPIDocs renders the OpenAPI spec and Scalar page and writes the two
// files under .nself/dist through the sink (apidocs.Generate's output, same
// bytes and modes).
func (st *buildState) writeAPIDocs(cfg apidocs.ApiDocsConfig, routes []apidocs.PluginRoute) error {
	spec, page, err := apidocs.Render(st.cfg.ProjectName, st.cfg.BaseDomain, cfg, routes)
	if err != nil {
		return fmt.Errorf("generating api docs: %w", err)
	}
	dist := filepath.Join(st.workdir, ".nself", "dist")
	if err := st.sink.MkdirAll(dist, 0755); err != nil {
		return fmt.Errorf("generating api docs: creating dist dir: %w", err)
	}
	if err := st.sink.WriteFile(filepath.Join(dist, "openapi.json"), spec, 0644); err != nil {
		return fmt.Errorf("generating api docs: writing openapi.json: %w", err)
	}
	if err := st.sink.WriteFile(filepath.Join(dist, "scalar.html"), page, 0644); err != nil {
		return fmt.Errorf("generating api docs: writing scalar.html: %w", err)
	}
	return nil
}
