package build

// Purpose: gives every plugin compose service the same fixed "core" env vars
// custom services already get (see coreEnvVars/fixedCoreEnvVars in
// internal/compose/custom_services.go) — project identity, Postgres, Hasura,
// and the PLUGIN_INTERNAL_SECRET/NOTIFY_INTERNAL_SECRET the CLI already
// generates (orchestrator_secrets.go) but never delivered to plugin
// containers. Confirmed on the 2026-09-21 E2E golden path run:
// testproject_ai crash-looped on "PLUGIN_INTERNAL_SECRET required" and
// testproject_mux on "MUX_ALLOWED_INSERT_TABLES must be set in production
// mode" (no ENV/NSELF_ENV reaching the container to signal dev mode) — the
// installed docker-compose.plugin.yml fragments only declared DATABASE_URL
// and PORT.
// Inputs: a plugin compose fragment's bytes + its name/port, or the vars map
// ComputePluginEnvVars builds for .nself/compose.env / .env.computed.
// Outputs: the (possibly rewritten) fragment bytes, or vars mutated in place.
// Constraints: fragments live in the GLOBAL plugin dir (~/.nself/plugins) and
// are shared across every project with the plugin installed, so
// project-specific values are injected as ${VAR} interpolation references —
// exactly like the pre-existing ${DOCKER_NETWORK}/${DATABASE_URL} refs — not
// literals; docker compose resolves them per-project from
// .nself/compose.env at container start. A key already authored in the
// fragment always wins — regardless of whether the fragment's environment:
// block uses map form ("KEY: value") or list form ("- KEY=value"); both are
// supported (see normalizeComposePluginCoreEnv and plugins_core_env_inject.go).

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ui"
)

// pluginCoreEnvKeys is the fixed, deterministically ordered set of env keys
// injected into every plugin service's environment: block. Order is fixed so
// re-normalizing an already-patched fragment is byte-stable.
var pluginCoreEnvKeys = []string{
	"ENV", "NSELF_ENV", "PROJECT_NAME", "COMPOSE_PROJECT_NAME",
	"DOCKER_NETWORK", "BASE_DOMAIN", "DATABASE_URL",
	"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_DB", "POSTGRES_USER", "POSTGRES_PASSWORD",
	"HASURA_GRAPHQL_ENDPOINT", "HASURA_GRAPHQL_ADMIN_SECRET",
	"PLUGIN_INTERNAL_SECRET", "NOTIFY_INTERNAL_SECRET",
	"SERVICE_NAME", "SERVICE_PORT",
}

// pluginCoreEnvValue returns the compose-fragment-literal text for key on a
// plugin named pluginName listening on port (0 if unknown — SERVICE_PORT is
// then omitted). Postgres/Hasura container hostnames and Hasura's internal
// port are fixed by the compose network topology, not project config (same
// reasoning as fixedCoreEnvVars), so those are plain literals; everything
// project-specific is a ${VAR} reference (see file header).
func pluginCoreEnvValue(key, pluginName string, port int) (string, bool) {
	switch key {
	case "ENV", "NSELF_ENV":
		return "${ENV}", true
	case "PROJECT_NAME":
		return "${PROJECT_NAME}", true
	case "COMPOSE_PROJECT_NAME":
		return "${COMPOSE_PROJECT_NAME}", true
	case "DOCKER_NETWORK":
		return "${DOCKER_NETWORK}", true
	case "BASE_DOMAIN":
		return "${BASE_DOMAIN}", true
	case "DATABASE_URL":
		return "${DATABASE_URL}", true
	case "POSTGRES_HOST":
		return "postgres", true
	case "POSTGRES_PORT":
		return `"5432"`, true
	case "POSTGRES_DB":
		return "${POSTGRES_DB}", true
	case "POSTGRES_USER":
		return "${POSTGRES_USER}", true
	case "POSTGRES_PASSWORD":
		return "${POSTGRES_PASSWORD}", true
	case "HASURA_GRAPHQL_ENDPOINT":
		return "http://hasura:8080/v1/graphql", true
	case "HASURA_GRAPHQL_ADMIN_SECRET":
		return "${HASURA_GRAPHQL_ADMIN_SECRET}", true
	case "PLUGIN_INTERNAL_SECRET":
		return "${PLUGIN_INTERNAL_SECRET}", true
	case "NOTIFY_INTERNAL_SECRET":
		return "${NOTIFY_INTERNAL_SECRET}", true
	case "SERVICE_NAME":
		return pluginName, true
	case "SERVICE_PORT":
		if port <= 0 {
			return "", false
		}
		return fmt.Sprintf("%q", fmt.Sprintf("%d", port)), true
	default:
		return "", false
	}
}

// pluginCoreEnvListLine renders "KEY=value" for a list-form environment:
// entry ("- KEY=value"). pluginCoreEnvValue's map-form values are sometimes
// wrapped in literal double quotes so YAML doesn't parse them as a number
// (POSTGRES_PORT: "5432", SERVICE_PORT: "3709") — list form's "- KEY=VALUE"
// is already a single YAML string, so a wrapping quote would become part of
// the value itself and must be stripped before use.
func pluginCoreEnvListLine(key, pluginName string, port int) (string, bool) {
	val, ok := pluginCoreEnvValue(key, pluginName, port)
	if !ok {
		return "", false
	}
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		val = val[1 : len(val)-1]
	}
	return key + "=" + val, true
}

// pluginEnvBlockRE matches an "environment:" key line at some indent,
// followed by nothing but a newline — the anchor for both map-form
// ("KEY: value") and list-form ("- KEY=value") child blocks, distinguished
// by scanEnvBlock. Mirrors shortNetworkListRE's single-service, single-block
// assumption in plugins_network_alias.go.
var pluginEnvBlockRE = regexp.MustCompile(`(?m)^([ \t]+)environment:[ \t]*\r?\n`)

// pluginEnvEmptyListRE matches an inline-empty "environment: []" — a shape
// distinct from pluginEnvBlockRE because there is no child block to scan.
var pluginEnvEmptyListRE = regexp.MustCompile(`(?m)^([ \t]+)environment:[ \t]*\[\][ \t]*\r?\n`)

// pluginEnvEntryKeyRE extracts the key name from a map-form environment:
// child line such as "      DATABASE_URL: ${DATABASE_URL}".
var pluginEnvEntryKeyRE = regexp.MustCompile(`^[ \t]+([A-Za-z_][A-Za-z0-9_]*):`)

// pluginEnvListEntryKeyRE extracts the key name from a trimmed list-form
// environment: child entry such as "- DATABASE_URL=${DATABASE_URL}" or a
// bare passthrough "- DATABASE_URL" (no value — inherited from the shell
// environment docker compose runs in).
var pluginEnvListEntryKeyRE = regexp.MustCompile(`^-[ \t]+([A-Za-z_][A-Za-z0-9_]*)(?:=.*)?$`)

// normalizeComposePluginCoreEnv ensures a plugin's compose service carries
// the full pluginCoreEnvKeys set. A key already present in the fragment's
// environment: block (any value, map or list form) is left untouched.
// Idempotent — re-running against an already-normalized fragment is a
// no-op, since DiscoverPluginComposeFiles calls this on every `nself build`.
//
// Both map form ("KEY: value") and list form ("- KEY=value") are supported
// — list form is not an edge case: 40+ installed fragments use it (every
// plugins-pro/paid/* bundle scaffolded from the shared template, plus free
// plugins such as cron). When the fragment has no environment: block yet,
// one is inserted immediately before the service's short-form "networks:"
// list — the same anchor normalizeComposeNetworkAliases uses for its
// "hostname:" injection. A shape this rewrite can't safely extend (an
// env_file-only fragment with no networks: anchor either, or a block using
// a YAML merge key "<<: *anchor" whose expanded keys we can't see) is left
// unchanged with a ui.Warn naming the plugin and file, rather than silently
// dropping the injection.
func normalizeComposePluginCoreEnv(content []byte, pluginDir, pluginName string) []byte {
	if pluginName == "" {
		return content
	}
	port := 0
	if m := readPluginManifest(pluginDir, pluginName); m != nil {
		port = m.Port
	}

	if loc := pluginEnvEmptyListRE.FindSubmatchIndex(content); loc != nil {
		return injectIntoEmptyListEnvBlock(content, loc, pluginName, port)
	}
	if loc := pluginEnvBlockRE.FindSubmatchIndex(content); loc != nil {
		shape, blockEnd, entryIndent, existing := scanEnvBlock(content, loc)
		switch shape {
		case envBlockList:
			return appendListEntries(content, blockEnd, entryIndent, existing, pluginName, port)
		case envBlockUnrecognised:
			warnUnrecognisedEnvBlock(pluginDir, pluginName)
			return content
		default: // envBlockMap
			return appendMapEntries(content, blockEnd, entryIndent, existing, pluginName, port)
		}
	}
	if out, ok := injectNewEnvBlock(content, pluginName, port); ok {
		return out
	}
	warnUnrecognisedEnvBlock(pluginDir, pluginName)
	return content
}

// warnUnrecognisedEnvBlock reports a plugin compose fragment whose
// environment configuration normalizeComposePluginCoreEnv cannot safely
// rewrite: an env_file-only fragment with no environment: block and no
// networks: short-list anchor to insert a new one at, or an environment:
// block using a YAML merge key ("<<: *anchor") whose expanded keys aren't
// visible to a text-level rewrite. The plugin ships without
// PLUGIN_INTERNAL_SECRET/NOTIFY_INTERNAL_SECRET and the rest of
// pluginCoreEnvKeys until its fragment is updated by hand.
func warnUnrecognisedEnvBlock(pluginDir, pluginName string) {
	path := filepath.Join(pluginDir, pluginName, pluginComposeFilename)
	ui.Warn(fmt.Sprintf(
		"plugin %q: %s has an environment configuration nself can't safely extend (env_file-only with no networks: anchor, or a YAML merge key) — add PLUGIN_INTERNAL_SECRET, NOTIFY_INTERNAL_SECRET and the other core env vars manually",
		pluginName, path))
}

// addPluginCoreEnvVars adds the project-specific values that the ${VAR}
// references normalizeComposePluginCoreEnv injects resolve to, into vars
// (merged by ComputePluginEnvVars into .nself/compose.env / .env.computed).
// nil cfg is a no-op, preserving ComputePluginEnvVars' pre-existing
// zero-cfg callers/tests. DOCKER_NETWORK, DATABASE_URL, POSTGRES_PASSWORD
// and HASURA_GRAPHQL_ADMIN_SECRET are already written elsewhere
// (WriteComposeEnv itself, and SecretEnvMap) so are deliberately not
// duplicated here; POSTGRES_HOST/PORT and HASURA_GRAPHQL_ENDPOINT are fixed
// literals in the fragment (see pluginCoreEnvValue), needing no entry here.
func addPluginCoreEnvVars(vars map[string]string, cfg *config.Config) {
	if cfg == nil {
		return
	}
	vars["ENV"] = cfg.Env
	vars["PROJECT_NAME"] = cfg.ProjectName
	vars["COMPOSE_PROJECT_NAME"] = cfg.ProjectName
	vars["BASE_DOMAIN"] = cfg.BaseDomain
	vars["POSTGRES_DB"] = cfg.Postgres.DB
	vars["POSTGRES_USER"] = cfg.Postgres.User
	if cfg.PluginSystem.InternalSecret != "" {
		vars["PLUGIN_INTERNAL_SECRET"] = cfg.PluginSystem.InternalSecret
	}
	if cfg.PluginConfig.NotifySecret != "" {
		vars["NOTIFY_INTERNAL_SECRET"] = cfg.PluginConfig.NotifySecret
	}
}
