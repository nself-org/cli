package nginx

// dynamic_resolve_test.go — regression coverage for the 2026-09-03 staging
// outage (auth.task.stack-two.nself.org 502 after ntask_auth was recreated).
//
// Purpose: prove every generated service route resolves its proxy target at
// request time (via the Docker embedded DNS resolver) instead of caching a
// container IP in a static `upstream {}` block that only re-resolves on a
// manual `nginx -s reload`.
// Inputs: a Config exercising core, optional, and fronted-stack (internal)
// routes.
// Outputs: pass/fail on whether any generated site conf still contains a
// static `upstream {}` block.
// Constraints: two of these routes (Mailpit, Functions) shipped without the
// dynamic-resolve pattern despite it already existing for Hasura/Auth/
// Storage/Admin/custom services — this test closes that gap generator-wide
// rather than per-route, so a new route can't reintroduce it.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// staticUpstreamBlock matches an actual nginx `upstream name { ... }` block
// declaration line — not merely the word "upstream" appearing in a comment.
var staticUpstreamBlock = regexp.MustCompile(`(?m)^\s*upstream\s+\S+\s*\{`)

// TestServiceRoutesResolveUpstreamDynamically verifies that no generated
// nginx site conf contains a static `upstream {}` block. A static block is
// resolved once when nginx loads and is never re-resolved for the life of
// the worker process, so a container recreate (nself restart, OOM, `docker
// system prune`, a plain rebuild) 502s every request through that route
// until someone notices and runs `nginx -s reload` by hand — exactly what
// happened to auth.task.stack-two.nself.org on 2026-09-03.
func TestServiceRoutesResolveUpstreamDynamically(t *testing.T) {
	cfg := &config.Config{BaseDomain: "stack-two.nself.org"}
	cfg, err := config.ApplyDefaults(cfg)
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}

	// Optional core-adjacent services (Mailpit, Functions) — these are the
	// two routes that were still emitting a static upstream block.
	cfg.Mailpit.Enabled = true
	cfg.Functions.Enabled = true
	cfg.Admin.Enabled = true
	cfg.Search.Enabled = true

	// A fronted-stack route, modeling nself-web's nginx proxying to
	// ntask_auth:4001 on staging — the exact route that 502ed.
	cfg.InternalRoutes = []config.InternalRoute{
		{Index: 1, Name: "task-auth", Subdomain: "auth.task", Target: "http://ntask_auth:4001"},
	}

	gen := NewGenerator(cfg, t.TempDir())
	files, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	checked := 0
	for path, content := range files {
		if !strings.HasPrefix(path, "nginx/sites/") {
			continue
		}
		checked++
		if staticUpstreamBlock.MatchString(content) {
			t.Errorf("%s contains a static `upstream {}` block — proxy target is cached at load time and will 502 after the target container is recreated until a manual `nginx -s reload`:\n%s", path, content)
		}
		if !strings.Contains(content, "set $up_") || !strings.Contains(content, "set $healthz_up") {
			t.Errorf("%s does not use the per-request dynamic-resolve pattern (set $up_<name> / $healthz_up)", path)
		}
	}

	if checked == 0 {
		t.Fatal("no nginx/sites/*.conf files were generated — test setup is broken")
	}
}

// TestNginxConfHasEmbeddedDNSResolver verifies nginx.conf.tmpl declares the
// Docker embedded DNS resolver (127.0.0.11) that the dynamic-resolve pattern
// in service.conf.tmpl depends on. Without this directive, `set $up_x
// http://svc:port; proxy_pass $up_x;` fails nginx startup with "no resolver
// defined to resolve svc".
func TestNginxConfHasEmbeddedDNSResolver(t *testing.T) {
	cfg := &config.Config{BaseDomain: "example.com"}
	cfg, err := config.ApplyDefaults(cfg)
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}

	gen := NewGenerator(cfg, t.TempDir())
	files, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	main := files["nginx/nginx.conf"]
	if !strings.Contains(main, "resolver 127.0.0.11") {
		t.Errorf("nginx/nginx.conf does not declare the Docker embedded DNS resolver (127.0.0.11); the dynamic-resolve proxy_pass pattern requires it:\n%s", main)
	}
}

// proxyVarAssignment matches the `set $<var> <target>;` lines that carry the
// dynamic proxy target, capturing the target so its scheme can be checked.
var proxyVarAssignment = regexp.MustCompile(`(?m)^\s*set\s+\$\S+\s+(\S+);`)

// TestProxyTargetsHaveExactlyOneScheme guards the second half of the
// dynamic-resolve pattern. The template prepended "http://" to every
// upstream, but an INTERNAL_ROUTE target already carries its own scheme
// (validateInternalRouteTarget rejects one without), so a fronted-stack
// route rendered `set $up_x http://http://ntask_auth:4001;`. nginx resolves
// a variable proxy_pass at request time, so that config loads cleanly and
// then fails every request against the host "http" — the same
// silent-until-traffic failure class as the stale-IP bug this file covers.
func TestProxyTargetsHaveExactlyOneScheme(t *testing.T) {
	cfg := &config.Config{BaseDomain: "stack-two.nself.org"}
	cfg, err := config.ApplyDefaults(cfg)
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}
	cfg.InternalRoutes = []config.InternalRoute{
		{Index: 1, Name: "task-auth", Subdomain: "auth.task", Target: "http://ntask_auth:4001"},
		{Index: 2, Name: "task-api", Subdomain: "api.task", Target: "https://ntask_hasura:8080"},
	}

	gen := NewGenerator(cfg, t.TempDir())
	files, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	checked := 0
	for path, content := range files {
		if !strings.HasPrefix(path, "nginx/sites/") {
			continue
		}
		for _, m := range proxyVarAssignment.FindAllStringSubmatch(content, -1) {
			target := m[1]
			checked++
			scheme := strings.Count(target, "://")
			if scheme != 1 {
				t.Errorf("%s: proxy target %q has %d schemes, want exactly 1 — nginx would resolve the wrong host at request time", path, target, scheme)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no `set $var <target>;` proxy assignments were found — test setup is broken")
	}

	// The https internal route must keep its scheme, not be rewritten to http.
	apiConf := files["nginx/sites/ir-task-api.conf"]
	if !strings.Contains(apiConf, "https://ntask_hasura:8080") {
		t.Errorf("ir-task-api.conf lost the https scheme of its target:\n%s", apiConf)
	}
}
