package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
	"github.com/nself-org/cli/internal/plugin/readiness"
)

// Purpose: doctor findings for contract:plugin.boot-migrations (ADR 0010).
//   - E120: an installed plugin ships migrations/ but its manifest lacks
//     migrations.apply: boot, so nothing applies the SQL at start.
//   - E118/E119: a running plugin that declares migrations reports fewer
//     applied than expected in /health (E118) or no migrations field (E119).
//
// Inputs: the plugin install dir (~/.nself/plugins), and a lookup of running
// plugin containers to their /health URL.
// Outputs: []CheckResult in section "plugins". A compliant plugin that is not
// running yields no result; a ready running plugin yields one pass.
// Constraints: one probe per plugin, no waiting (install flows wait through
// readiness.Wait); nothing is applied; an unreadable manifest or an
// unanswerable lookup is reported, never skipped.

// runningPlugin is a plugin container with a reachable /health URL.
type runningPlugin struct {
	Name      string
	HealthURL string
}

// runningLookup returns running plugins by plugin name.
type runningLookup func(ctx context.Context) (map[string]runningPlugin, error)

// PluginMigrationChecks is the DeepChecks entry point.
func PluginMigrationChecks(ctx context.Context, pluginDir string) []CheckResult {
	return pluginMigrationChecks(ctx, pluginDir, dockerRunningPlugins)
}

func pluginMigrationChecks(ctx context.Context, pluginDir string, lookup runningLookup) []CheckResult {
	var results []CheckResult
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return []CheckResult{migWarn("PLUGIN-MIGRATIONS-dir", fmt.Sprintf("cannot read plugin directory %s: %v", pluginDir, err))}
	}
	var declaring []string
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		name := ent.Name()
		dir := filepath.Join(pluginDir, name)
		raw, rerr := os.ReadFile(filepath.Join(dir, "plugin.json"))
		if errors.Is(rerr, os.ErrNotExist) {
			continue // not an installed plugin
		}
		if rerr != nil {
			results = append(results, migWarn("PLUGIN-MIGRATIONS-"+name, fmt.Sprintf("plugin %s: cannot read plugin.json: %v", name, rerr)))
			continue
		}
		m, perr := manifestv2.ParseQuiet(raw)
		if perr != nil {
			results = append(results, migWarn("PLUGIN-MIGRATIONS-"+name, fmt.Sprintf("plugin %s: cannot check migrations, plugin.json invalid: %v", name, perr)))
			continue
		}
		migDir := "migrations"
		if m.Migrations != nil && m.Migrations.Dir != "" {
			migDir = m.Migrations.Dir
		}
		st, serr := os.Stat(filepath.Join(dir, migDir))
		hasFiles := serr == nil && st.IsDir()
		if serr != nil && !errors.Is(serr, os.ErrNotExist) {
			results = append(results, migWarn("PLUGIN-MIGRATIONS-"+name, fmt.Sprintf("plugin %s: cannot stat %s: %v", name, migDir, serr)))
			continue
		}
		if hasFiles && (m.Migrations == nil || m.Migrations.Apply != "boot") {
			e := errs.Newf("E120", "plugin %s ships %s/ but plugin.json has no migrations.apply: boot", name, migDir)
			results = append(results, CheckResult{Section: "plugins", Name: "PLUGIN-MIGRATIONS-BOOT-" + name, Status: "fail",
				Message: "E120: " + e.What, FixCmd: e.Fix})
		}
		if hasFiles || m.Migrations != nil {
			declaring = append(declaring, name)
		}
	}
	if len(declaring) == 0 {
		return results
	}
	sort.Strings(declaring)
	running, lerr := lookup(ctx)
	if lerr != nil {
		return append(results, migWarn("PLUGIN-MIGRATIONS-running", fmt.Sprintf("cannot list running plugins to check migrations: %v", lerr)))
	}
	for _, name := range declaring {
		rp, ok := running[name]
		if !ok {
			continue
		}
		results = append(results, probeMigrations(ctx, rp))
	}
	return results
}

func probeMigrations(ctx context.Context, rp runningPlugin) CheckResult {
	id := "PLUGIN-MIGRATIONS-" + rp.Name
	p, err := readiness.Probe(ctx, nil, rp.Name, rp.HealthURL)
	switch {
	case err == nil && p.Ready():
		return CheckResult{Section: "plugins", Name: id, Status: "pass",
			Message: fmt.Sprintf("plugin %s: migrations applied %d of %d", rp.Name, p.Applied, p.Expected)}
	case err == nil:
		e := errs.Newf("E118", "plugin %s: migrations applied %d of %d", rp.Name, p.Applied, p.Expected)
		return CheckResult{Section: "plugins", Name: id, Status: "fail", Message: "E118: " + e.What,
			FixCmd: fmt.Sprintf("docker logs nself_%s", rp.Name)}
	case readiness.IsUnreachable(err):
		return migWarn(id, fmt.Sprintf("plugin %s: cannot read migrations status: %v", rp.Name, err))
	default:
		var ce *errs.CLIError
		if errors.As(err, &ce) {
			return CheckResult{Section: "plugins", Name: id, Status: "fail", Message: ce.Code + ": " + ce.What, FixCmd: ce.Fix}
		}
		return migWarn(id, err.Error())
	}
}

func migWarn(name, msg string) CheckResult {
	return CheckResult{Section: "plugins", Name: name, Status: "warn", Message: msg}
}

// dockerRunningPlugins maps running plugin containers (label nself.plugin) to
// http://127.0.0.1:<host port>/health, the probe PluginHealthChecks uses.
func dockerRunningPlugins(ctx context.Context) (map[string]runningPlugin, error) {
	out, err := exec.CommandContext(ctx, "docker", "ps", "--filter", "label=nself.plugin", "--format", "{{.Names}}").Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	found := map[string]runningPlugin{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		container := strings.TrimSpace(line)
		if container == "" {
			continue
		}
		name := strings.TrimPrefix(container, "nself_")
		port, perr := resolveHostPort(ctx, container)
		if perr != nil {
			return nil, perr
		}
		n, cerr := strconv.Atoi(port)
		if cerr != nil || n < 1024 || n > 65535 {
			continue // no host port: PluginHealthChecks already warns
		}
		found[name] = runningPlugin{Name: name, HealthURL: fmt.Sprintf("http://127.0.0.1:%d/health", n)}
	}
	return found, nil
}
