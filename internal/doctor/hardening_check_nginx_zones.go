package doctor

// hardening_check_nginx_zones.go — SEC-HARDENING-06: nginx rate-limit zones
// for the auth and API surfaces. Split out of hardening_check_auth_net.go
// (CLI-R12) as a pure move.
//
// Inputs: a context (for the docker-exec fallback) and the project directory.
// Outputs: a single CheckResult — pass/warn/fail with remediation hint.
// Constraints: depends on hardeningSection, defined elsewhere in this package.
// SPORT: cli/internal/doctor — decomposed from hardening_check.go (T-E2-06).
//
// Detection strategy (cli#379): the default `nself build` nginx layout is
// one `server {}` block per service, named by `server_name
// <route>.<domain>` (internal/nginx/templates/service.conf.tmpl) — auth's
// route defaults to "auth" (routes_core.go), Hasura/GraphQL's to "api", and
// the shipped ping-api example custom service to "ping"
// (internal/setup/setup_env_files.go: CS_1_ROUTE=ping). A hand-rolled or
// pre-P6-E2-W2-S3-T20 config may instead rate-limit the whole server block
// (`limit_req` inside `location /`) without ever mentioning the literal
// paths /auth/login or /api/ anywhere in the file — the original
// literal-path substring scan can never pass that shape even though the
// service genuinely is rate-limited. So this check now looks for EITHER
// signal, per file:
//  1. Service identity: split the file into server{} blocks, read each
//     block's server_name, and if a block whose first hostname label
//     matches a known auth/API route name also contains a `limit_req`
//     directive anywhere inside it (location / or a path-scoped location),
//     that half is satisfied.
//  2. Literal path fallback (original behavior, kept unweakened): the file
//     contains both "limit_req" and the literal "/auth/login" or "/api/"
//     path string, for hand-written gateway configs that route by path
//     rather than by server_name.
// A config with no limit_req anywhere still fails both signals. A
// limit_req confined to an unrelated server block (e.g. only the "static"
// frontend route) satisfies neither signal, since that block's server_name
// matches no known auth/API identity and the file lacks the literal paths.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// nginxAuthRouteNames are the first-label server_name hostnames that
// identify a server block as fronting the auth service. "auth" is the
// generator's default (routes_core.go authRoute) and survives app-prefixed
// routes too (appPrefixedRoute produces "auth.<appname>", whose first label
// is still "auth").
var nginxAuthRouteNames = map[string]bool{
	"auth": true,
}

// nginxAPIRouteNames are the first-label server_name hostnames that
// identify a server block as fronting the API/GraphQL surface. "api" is
// Hasura's default route (routes_core.go hasuraRoute), "hasura" and
// "graphql" cover an explicitly-renamed Hasura route, and "ping"/"ping-api"
// cover the shipped ping-api custom-service example
// (CS_1_NAME=ping-api, CS_1_ROUTE=ping in setup_env_files.go) — the same
// service cli#379 named as already being rate-limited on staging.
var nginxAPIRouteNames = map[string]bool{
	"api":      true,
	"hasura":   true,
	"graphql":  true,
	"ping":     true,
	"ping-api": true,
}

// nginxServerBlockRe finds the start of each top-level `server { ... }`
// block so its contents can be brace-matched out of the surrounding file.
var nginxServerBlockRe = regexp.MustCompile(`(?m)^\s*server\s*\{`)

// nginxServerNameRe extracts the hostname(s) from a `server_name ...;`
// directive inside a server block.
var nginxServerNameRe = regexp.MustCompile(`server_name\s+([^;]+);`)

// nginxServerBlock is one parsed `server {}` block: its declared
// server_name hostnames and whether it contains a limit_req directive
// anywhere in its body (location / or a path-scoped location).
type nginxServerBlock struct {
	serverNames []string
	hasLimitReq bool
}

// extractNginxServerBlocks splits nginx config content into its top-level
// server{} blocks via brace counting, so a limit_req found in one service's
// block is never misattributed to another. Nested blocks (location {})
// share the same brace depth as their parent, so counting to zero finds
// the correct outer closing brace regardless of nesting.
func extractNginxServerBlocks(content string) []nginxServerBlock {
	var blocks []nginxServerBlock
	pos := 0
	for {
		loc := nginxServerBlockRe.FindStringIndex(content[pos:])
		if loc == nil {
			break
		}
		start := pos + loc[0]
		braceIdx := pos + loc[1] - 1 // index of the opening '{'
		depth := 1
		i := braceIdx + 1
		for i < len(content) && depth > 0 {
			switch content[i] {
			case '{':
				depth++
			case '}':
				depth--
			}
			i++
		}
		body := content[start:i]
		pos = i

		var names []string
		for _, m := range nginxServerNameRe.FindAllStringSubmatch(body, -1) {
			for _, h := range strings.Fields(m[1]) {
				names = append(names, strings.ToLower(h))
			}
		}
		blocks = append(blocks, nginxServerBlock{
			serverNames: names,
			hasLimitReq: strings.Contains(body, "limit_req"),
		})
		if pos >= len(content) {
			break
		}
	}
	return blocks
}

// nginxHostnameFirstLabel returns the lowercased first dot-separated label
// of a server_name hostname, e.g. "auth.myapp.staging.nself.org" -> "auth".
func nginxHostnameFirstLabel(hostname string) string {
	if i := strings.IndexByte(hostname, '.'); i >= 0 {
		return hostname[:i]
	}
	return hostname
}

// scanNginxContentForRateZones applies both detection signals (service
// identity by server_name, and the literal-path fallback) to one file's
// raw nginx config text, returning which of the auth/API halves it
// satisfies.
func scanNginxContentForRateZones(content string) (authZone, apiZone bool) {
	// Signal 1: service identity — a server{} block whose server_name
	// identifies it as auth or API AND that contains a limit_req anywhere
	// in its body (server-wide `location /` zone or a path-scoped one).
	for _, block := range extractNginxServerBlocks(content) {
		if !block.hasLimitReq {
			continue
		}
		for _, name := range block.serverNames {
			label := nginxHostnameFirstLabel(name)
			if nginxAuthRouteNames[label] {
				authZone = true
			}
			if nginxAPIRouteNames[label] {
				apiZone = true
			}
		}
	}

	// Signal 2: literal path fallback (original behavior, unweakened) —
	// hand-written gateway configs that route by literal path instead of
	// by server_name.
	if strings.Contains(content, "/auth/login") && strings.Contains(content, "limit_req") {
		authZone = true
	}
	if strings.Contains(content, "/api/") && strings.Contains(content, "limit_req") {
		apiZone = true
	}

	return authZone, apiZone
}

// checkHardeningNginxRateZones verifies nginx has limit_req_zone + limit_req
// directives covering the auth and API surfaces, first by scanning local
// config files, then by grepping inside the running nginx container as a
// fallback. See the file-level doc comment for the two detection signals.
//
// Fronted deployments (cli#380/cli#371) need a different directory than
// <projectDir>/nginx/** — see planNginxAudit (hardening_check_nginx_fronted_dir.go).
func checkHardeningNginxRateZones(ctx context.Context, projectDir string) CheckResult {
	const checkID = "SEC-HARDENING-06"

	plan := planNginxAudit(projectDir)
	if plan.skip != nil {
		return *plan.skip
	}
	auditDir := plan.auditDir

	// Search <auditDir>/nginx/conf.d/, /sites/ and /nginx.conf for
	// limit_req_zone + limit_req directives covering the auth and API
	// surfaces.
	// auditDir is already the served root (planNginxAudit); the one resolver
	// (D-0045) names its nginx dir. No error is possible for an empty frontedBy.
	auditedNginxDir, _ := nginxtopo.ServedNginxDir(auditDir, "")
	nginxDirs := []string{
		filepath.Join(auditedNginxDir, "conf.d"),
		filepath.Join(auditedNginxDir, "sites"),
		filepath.Join(auditedNginxDir, "nginx.conf"),
	}

	hasAuthZone := false
	hasAPIZone := false

	for _, root := range nginxDirs {
		info, err := os.Stat(root)
		if err != nil {
			continue
		}

		var files []string
		if info.IsDir() {
			entries, err := os.ReadDir(root)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					files = append(files, filepath.Join(root, e.Name()))
				}
			}
		} else {
			files = []string{root}
		}

		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			authZone, apiZone := scanNginxContentForRateZones(string(data))
			if authZone {
				hasAuthZone = true
			}
			if apiZone {
				hasAPIZone = true
			}
		}
	}

	// Fallback: inspect nginx container config if local files not found.
	// Skipped for fronted projects — they generate no nginx container of
	// their own to exec into (internal/build/detection.go), so this would
	// only ever exec into a nonexistent or unrelated container.
	if !plan.skipDockerFallback && (!hasAuthZone || !hasAPIZone) {
		nginxContainer := health.ContainerName(resolveProjectName(projectDir), "nginx")
		cmd := exec.CommandContext(ctx, "docker", "exec", nginxContainer,
			"sh", "-c", "cat /etc/nginx/nginx.conf /etc/nginx/conf.d/* /etc/nginx/sites/* 2>/dev/null")
		out, err := cmd.Output()
		if err == nil {
			authZone, apiZone := scanNginxContentForRateZones(string(out))
			if authZone {
				hasAuthZone = true
			}
			if apiZone {
				hasAPIZone = true
			}
		}
	}

	// auditedNginxDir (above) names what this run read, so the result is falsifiable.

	switch {
	case hasAuthZone && hasAPIZone:
		return CheckResult{
			Section: hardeningSection,
			Name:    checkID,
			Status:  "pass",
			Message: fmt.Sprintf("SEC-HARDENING-06: nginx rate-limit zones set for the auth and API services (audited %s)", auditedNginxDir),
		}
	case !hasAuthZone && !hasAPIZone:
		return CheckResult{
			Section: hardeningSection,
			Name:    checkID,
			Status:  "fail",
			Message: fmt.Sprintf("SEC-HARDENING-06: nginx missing rate-limit zones for the auth and API services (audited %s) — add limit_req_zone directives", auditedNginxDir),
			FixCmd:  "See nself.org/docs/security/nginx-rate-limiting",
		}
	default:
		missing := "the API service (e.g. hasura/api.<domain>)"
		if !hasAuthZone {
			missing = "the auth service (auth.<domain>)"
		}
		return CheckResult{
			Section: hardeningSection,
			Name:    checkID,
			Status:  "warn",
			Message: fmt.Sprintf("SEC-HARDENING-06: nginx rate-limit zone missing for %s (audited %s) — add a limit_req directive", missing, auditedNginxDir),
			FixCmd:  "See nself.org/docs/security/nginx-rate-limiting",
		}
	}
}
