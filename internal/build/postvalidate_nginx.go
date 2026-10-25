package build

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Purpose: post-build nginx config syntax verification (`nginx -t`), plus the
// nginx.conf path resolver it depends on. Split out of postvalidate.go
// (CLI-R12) as a pure move.
// Inputs: the nginx config dir (nginx/ or nginx/sites/) and the shared
// *PostValidateResult to append findings to.
// Outputs: appends to result.Errors/Warnings; never returns a Go error.
// Constraints: skips (warn, not fail) when nginx is not installed locally.

// checkServerNameUniqueness reports every FQDN claimed by more than one
// server block, on the same listen port, across the generated
// nginx/sites/ directory — naming both source files.
//
// nginx does not treat this as a config error. It logs
// `conflicting server name "x" on 0.0.0.0:443, ignored`, keeps whichever
// block it loaded last, and still prints "syntax is ok" — so
// checkNginxSyntax below passes and one of the two routes is silently
// dead. That is how api.stack-two.nself.org ended up served by an
// unintended block on 2026-09-03.
//
// Three separate writers populate nginx/sites/ in one build — the nginx
// generator (Step 7), plugin route injection (Step 7.1), and the api-docs
// site conf (Step 11) — and none can see the others' output. This runs
// after all of them, the only point where the whole directory is visible.
//
// The check is keyed on port AND name because that is nginx's own rule: a
// name served on :80 by one file and on :443 by another is a legitimate
// split, not a conflict, and failing a build over it would be worse than
// the bug being fixed.
func checkServerNameUniqueness(nginxConfDir string, result *PostValidateResult) {
	sitesDir := nginxConfDir
	if filepath.Base(sitesDir) != "sites" {
		sitesDir = filepath.Join(nginxConfDir, "sites")
	}

	entries, err := os.ReadDir(sitesDir)
	if err != nil {
		// No generated sites directory is not a finding — a stack fronted
		// by another stack's nginx legitimately generates none.
		return
	}

	// Sorted so the file reported as the first claimant, and therefore the
	// exact wording of the error, is identical on every machine and run.
	// os.ReadDir order is not guaranteed across platforms, and a conflict
	// that names a different file each run is not actionable.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".conf") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	// claimedBy maps "<port>|<server_name>" to the first file to claim it.
	claimedBy := make(map[string]string)

	for _, name := range names {
		data, readErr := os.ReadFile(filepath.Join(sitesDir, name))
		if readErr != nil {
			continue
		}
		for _, block := range parseServerBlocks(string(data)) {
			ports := block.Ports
			if len(ports) == 0 {
				ports = []string{defaultListenPort}
			}
			for _, serverName := range block.ServerNames {
				if serverName == "_" {
					continue // the catch-all default server, intentionally shared
				}
				for _, port := range ports {
					key := port + "|" + serverName
					first, dup := claimedBy[key]
					if !dup {
						claimedBy[key] = name
						continue
					}
					if first == name {
						continue // same file, e.g. its own :80 and :443 blocks
					}
					result.NginxValid = false
					result.Errors = append(result.Errors, fmt.Sprintf(
						"nginx server_name %q on port %s is claimed by both %s and %s — nginx logs \"conflicting server name\" and silently serves only one of them; rename one route or remove the duplicate",
						serverName, port, first, name))
				}
			}
		}
	}
}

// checkNginxSyntax runs `nginx -t -c <nginx.conf>` if the nginx binary is
// available in PATH and the main nginx.conf can be found. Skips with a
// warning when nginx is not installed locally — that is not an error,
// since users may not have nginx on their development machine.
//
// The nginxConfDir parameter is expected to be nginx/sites/ (or nginx/).
// We look for nginx.conf one level up (the standard layout).
func checkNginxSyntax(nginxConfDir string, result *PostValidateResult) {
	// Locate nginx binary.
	nginxBin, err := exec.LookPath("nginx")
	if err != nil {
		result.Warnings = append(result.Warnings,
			"nginx binary not found in PATH — skipping nginx syntax check (install nginx locally to enable)")
		return
	}

	// Determine path to main nginx.conf. The caller passes nginx/sites/,
	// so the main conf lives in the parent directory.
	nginxConf := findNginxConf(nginxConfDir)
	if nginxConf == "" {
		result.Warnings = append(result.Warnings,
			"nginx.conf not found — skipping nginx syntax check")
		return
	}

	// Run nginx -t with a short timeout so a hung binary never blocks the build.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// nginx -t writes its output to stderr by convention.
	cmd := exec.CommandContext(ctx, nginxBin, "-t", "-c", nginxConf)
	out, err := cmd.CombinedOutput()

	// nginx -t validates config SYNTAX and then attempts runtime probes
	// (e.g. opening the pid file from the `pid` directive). When run as a
	// non-root user without a writable pid path — the norm in CI and most
	// local `nself build` runs — nginx prints "syntax is ok" and then exits
	// non-zero with "[emerg] open() /var/run/nginx.pid failed (Permission
	// denied)". That is a runtime-permission artifact, not a config-syntax
	// error, so we gate on the syntax verdict in the output rather than the
	// exit code alone: if nginx reports the syntax OK, the check passes.
	outStr := string(out)
	if strings.Contains(outStr, "syntax is ok") {
		return
	}
	if err != nil {
		result.NginxValid = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("nginx syntax check failed: %v\n%s", err, strings.TrimSpace(outStr)))
		return
	}

	// Success — nginx -t exits 0 and prints "syntax is ok".
}

// findNginxConf searches for nginx.conf relative to the provided directory.
// nginxConfDir is expected to be either nginx/ or nginx/sites/; we check
// common locations and return the first one that exists.
func findNginxConf(nginxConfDir string) string {
	candidates := []string{
		// If caller passed nginx/sites/, the conf is one level up.
		filepath.Join(nginxConfDir, "..", "nginx.conf"),
		// If caller passed nginx/ directly.
		filepath.Join(nginxConfDir, "nginx.conf"),
	}

	for _, candidate := range candidates {
		clean := filepath.Clean(candidate)
		if _, err := os.Stat(clean); err == nil {
			abs, err := filepath.Abs(clean)
			if err == nil {
				return abs
			}
			return clean
		}
	}

	return ""
}
