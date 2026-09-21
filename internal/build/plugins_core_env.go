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
// fragment always wins.

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/nself-org/cli/internal/config"
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

// pluginEnvBlockRE matches a map-form "environment:" key line at some indent
// (the shape every installed plugin fragment uses — mirrors
// shortNetworkListRE's single-service, single-block assumption in
// plugins_network_alias.go).
var pluginEnvBlockRE = regexp.MustCompile(`(?m)^([ \t]+)environment:[ \t]*\r?\n`)

// pluginEnvEntryKeyRE extracts the key name from an environment: child line
// such as "      DATABASE_URL: ${DATABASE_URL}".
var pluginEnvEntryKeyRE = regexp.MustCompile(`^[ \t]+([A-Za-z_][A-Za-z0-9_]*):`)

// normalizeComposePluginCoreEnv ensures a plugin's compose service carries
// the full pluginCoreEnvKeys set. A key already present in the fragment's
// environment: block (any value) is left untouched. Idempotent — re-running
// against an already-normalized fragment is a no-op, since
// DiscoverPluginComposeFiles calls this on every `nself build`.
//
// When the fragment has no map-form environment: block yet, one is inserted
// immediately before the service's short-form "networks:" list — the same
// anchor normalizeComposeNetworkAliases uses for its "hostname:" injection.
// A fragment with neither block to anchor on, or whose environment: block
// uses list form ("- KEY=VALUE") rather than map form, is left unchanged:
// no plugin fragment observed in the wild takes either shape, and silently
// mixing map entries into a list block would produce invalid YAML.
func normalizeComposePluginCoreEnv(content []byte, pluginDir, pluginName string) []byte {
	if pluginName == "" {
		return content
	}
	port := 0
	if m := readPluginManifest(pluginDir, pluginName); m != nil {
		port = m.Port
	}

	if loc := pluginEnvBlockRE.FindSubmatchIndex(content); loc != nil {
		return injectIntoExistingEnvBlock(content, loc, pluginName, port)
	}
	return injectNewEnvBlock(content, pluginName, port)
}

// injectIntoExistingEnvBlock appends missing core env keys at the end of an
// already-present environment: map block. loc is the pluginEnvBlockRE match;
// loc[2]:loc[3] is the captured "environment:" indent, loc[1] the offset
// right after its trailing newline.
func injectIntoExistingEnvBlock(content []byte, loc []int, pluginName string, port int) []byte {
	envIndent := string(content[loc[2]:loc[3]])
	entryIndent := envIndent + "  "
	entryIndentB := []byte(entryIndent)

	existing := map[string]bool{}
	pos := loc[1]
	blockEnd := pos
	for _, line := range bytes.SplitAfter(content[pos:], []byte("\n")) {
		if len(line) == 0 || !bytes.HasPrefix(line, entryIndentB) {
			break
		}
		if after := bytes.TrimPrefix(line, entryIndentB); len(after) > 0 && after[0] == '-' {
			// List-form environment — a different shape than any observed
			// real fragment. Leave untouched rather than risk mixing map and
			// list syntax under the same key.
			return content
		}
		if m := pluginEnvEntryKeyRE.FindSubmatch(line); m != nil {
			existing[string(m[1])] = true
		}
		blockEnd += len(line)
	}

	var toAdd bytes.Buffer
	for _, key := range pluginCoreEnvKeys {
		if existing[key] {
			continue
		}
		if val, ok := pluginCoreEnvValue(key, pluginName, port); ok {
			fmt.Fprintf(&toAdd, "%s%s: %s\n", entryIndent, key, val)
		}
	}
	if toAdd.Len() == 0 {
		return content
	}

	var out bytes.Buffer
	out.Grow(len(content) + toAdd.Len())
	out.Write(content[:blockEnd])
	out.Write(toAdd.Bytes())
	out.Write(content[blockEnd:])
	return out.Bytes()
}

// injectNewEnvBlock inserts a brand-new environment: block, anchored
// immediately before the service's short-form "networks:" list (see
// shortNetworkListRE in plugins_network_alias.go), for a fragment that has
// no environment: block of its own yet.
func injectNewEnvBlock(content []byte, pluginName string, port int) []byte {
	match := shortNetworkListRE.FindSubmatchIndex(content)
	if match == nil {
		return content // no anchor to insert at — leave untouched
	}
	svcIndent := string(content[match[2]:match[3]])
	entryIndent := svcIndent + "  "

	var block bytes.Buffer
	fmt.Fprintf(&block, "%senvironment:\n", svcIndent)
	for _, key := range pluginCoreEnvKeys {
		if val, ok := pluginCoreEnvValue(key, pluginName, port); ok {
			fmt.Fprintf(&block, "%s%s: %s\n", entryIndent, key, val)
		}
	}

	var out bytes.Buffer
	out.Grow(len(content) + block.Len())
	out.Write(content[:match[0]])
	out.Write(block.Bytes())
	out.Write(content[match[0]:])
	return out.Bytes()
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
