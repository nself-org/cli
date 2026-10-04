package build

// nginx_sites_dir.go — decides which on-disk directory `nself build` writes
// generated per-service nginx site confs (nginx/sites/*.conf) into.
//
// Purpose: cli#385 — a project with NGINX_FRONTED_BY set generates no nginx
// container of its own (see detection.go: DetectServices omits "nginx" and
// compose.Generator skips the nginx service entirely when FrontedBy is
// set), so <workdir>/nginx/sites/ is never mounted into any running nginx.
// Before this fix, `nself build` still wrote there unconditionally — a
// staging box was found with the generator's output at
// "<checkout>/backend/nginx/sites" while the running nginx (belonging to
// the fronting stack) served from "/opt/nself-web/nginx/sites", an entirely
// different tree an operator's reconciliation step could copy stale/missing
// confs from without any warning. This targets the confs at the directory
// the fronting stack's own nginx actually reads from when that stack's
// layout can be confirmed, and refuses the build otherwise rather than
// silently writing a tree nginx never reads.
// Inputs: workdir (this project's resolved nSelf root) and
// cfg.Nginx.FrontedBy (NGINX_FRONTED_BY; empty for the default, unfronted
// case).
// Outputs: resolveNginxSitesDir returns the directory to write nginx/sites/
// *.conf files into, or an error naming both the project's own (unused)
// directory and the parent directory that was checked and did not resolve.
// Constraints: delegates to internal/nginxtopo.ServedNginxDir (P7-LIVE-02,
// D-0045) — the one served-dir resolver every nginx/ssl writer shares —
// rather than inventing a second FrontedBy-to-path mechanism. Only the
// success path delegates; the refusal text is unchanged (golden).

import (
	"fmt"
	"path/filepath"

	"github.com/nself-org/cli/internal/nginxtopo"
)

// resolveNginxSitesDir returns the directory `nself build` should write
// generated nginx/sites/*.conf route files into for workdir.
//
// When frontedBy is empty, behavior is unchanged: <workdir>/nginx/sites.
// When frontedBy is set, this project's own nginx/sites/
// is never read by any running nginx, so the fronting stack's own nginx
// directory is targeted instead — resolved via the same "this project lives
// directly under the fronting stack's directory, conventionally as
// 'backend'" convention the SEC-HARDENING-06 doctor check trusts. When that
// convention cannot be confirmed, this returns an error rather than
// guessing: the caller must not write a tree nginx never reads.
func resolveNginxSitesDir(workdir, frontedBy string) (string, error) {
	// Both answers come from nginxtopo (D-0045): the served resolver for the
	// fronted success path, and the own-topology answer (frontedBy "") for the
	// project's unused directory named in the refusal.
	nginxDir, err := nginxtopo.ServedNginxDir(workdir, frontedBy)
	if err == nil {
		return filepath.Join(nginxDir, "sites"), nil
	}

	ownNginxDir, _ := nginxtopo.ServedNginxDir(workdir, "")
	ownDir := filepath.Join(ownNginxDir, "sites")
	// Paths are interpolated with %s, not %q: on Windows %q doubles
	// each backslash in the path separator, which breaks any caller
	// (including tests) that looks for the literal path inside this
	// message via strings.Contains — the same class of bug documented
	// in internal/doctor/hardening_check_nginx_fronted_test.go's
	// messageNamesPath helper. Using %s here avoids needing that
	// tolerance at all.
	parent := filepath.Dir(workdir)
	return "", fmt.Errorf(
		"refusing to generate nginx site configs: NGINX_FRONTED_BY=%q names another "+
			"stack's nginx as this project's ingress, so %s is never read by any running "+
			"nginx (a fronted project generates no nginx container of its own) — and %s "+
			"could not be confirmed as %[1]q's own directory (its basename must equal "+
			"NGINX_FRONTED_BY); lay this project out as \"backend\" directly under "+
			"%[1]s's own directory, or unset NGINX_FRONTED_BY if this project fronts "+
			"itself",
		frontedBy, ownDir, parent)
}
