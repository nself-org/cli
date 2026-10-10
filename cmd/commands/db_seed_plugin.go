package commands

// Purpose: `nself db seed --plugin <name>` and `--all-plugins`. Runs the
// idempotent argv a plugin declares in manifest v2 `seed.command`, inside the
// plugin's own compose service container (docker exec funnel, no host shell).
// Inputs: --plugin, --all-plugins, --force, the global --json flag, the
// installed plugin directory and the project config.
// Outputs: human lines, or one cli.json-envelope (data.plugins[]{name,result}).
// Constraints: a prod-class env (anything but dev/local/development/test)
// refuses with E403 unless --force; a plugin without seed.command is E127
// under --plugin and skipped under --all-plugins; the seed argv is never
// interpolated or run on the host.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
	"github.com/nself-org/cli/internal/seed"
	"github.com/spf13/cobra"
)

// Seams for tests.
var (
	pluginSeedConfig  = loadProjectConfig
	pluginSeedDir     = resolvePluginDir
	pluginSeedRuntime = func(projectDir, project string) seed.PluginRuntime {
		return seed.DockerRuntime(projectDir, project)
	}
)

const pluginComposeDefault = "docker-compose.plugin.yml"

var pluginSeedNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// pluginSeedResult is one data.plugins[] entry.
type pluginSeedResult struct {
	Name   string `json:"name"`
	Result string `json:"result"` // seeded | skipped
}

type pluginSeedData struct {
	Plugins []pluginSeedResult `json:"plugins"`
}

func init() {
	dbSeedCmd.Flags().String("plugin", "", "Run this plugin's seed.command inside its service container")
	dbSeedCmd.Flags().Bool("all-plugins", false, "Run every installed plugin's seed.command")
	dbSeedCmd.Flags().Bool("force", false, "Allow plugin seeds on a prod-class env")

	legacy := dbSeedCmd.RunE
	dbSeedCmd.RunE = func(cmd *cobra.Command, args []string) error {
		one, _ := cmd.Flags().GetString("plugin")
		all, _ := cmd.Flags().GetBool("all-plugins")
		if one == "" && !all {
			if err := legacy(cmd, args); err != nil || !jsonEnvelopeOn(cmd) {
				return err
			}
			return emitEnv(cmd, pluginSeedData{Plugins: []pluginSeedResult{}}, false)
		}
		return runDBSeedPlugin(cmd, args, one, all)
	}
}

// pluginSeedEnvAllowed reports whether env is a dev/local-class environment.
func pluginSeedEnvAllowed(env string) bool {
	switch strings.ToLower(env) {
	case "", "dev", "development", "local", "test":
		return true
	}
	return false
}

func runDBSeedPlugin(cmd *cobra.Command, args []string, one string, all bool) error {
	if one != "" && all {
		return errs.New("E401", "--plugin and --all-plugins cannot be combined")
	}
	if len(args) > 0 {
		return errs.New("E401", "a seed file argument cannot be combined with --plugin or --all-plugins")
	}
	cfg, err := pluginSeedConfig()
	if err != nil {
		return err
	}
	force, _ := cmd.Flags().GetBool("force")
	if !pluginSeedEnvAllowed(cfg.Env) && !force {
		return errs.Newf("E403", "refusing to run plugin seeds on env %q", cfg.Env).
			WithWhy("plugin seeds write data and this is not a dev or local environment").
			WithFix("Run against a dev stack, or pass --force if you mean to seed this environment.")
	}
	projectDir, _ := os.Getwd()
	rt := pluginSeedRuntime(projectDir, cfg.ProjectName)
	pluginDir := pluginSeedDir()

	names := []string{one}
	if all {
		if names, err = installedPluginNames(pluginDir); err != nil {
			return err
		}
	}
	var results []pluginSeedResult
	var failed []string
	var firstErr error
	for _, name := range names {
		r, rerr := seedOnePlugin(cmd.Context(), rt, pluginDir, name, all)
		if rerr != nil {
			failed = append(failed, name)
			if firstErr == nil {
				firstErr = rerr
			}
			if !all {
				return rerr
			}
			continue
		}
		results = append(results, r)
	}
	if len(failed) > 0 {
		return errs.Wrap("E250", "plugin seed failed for: "+strings.Join(failed, ", "), firstErr)
	}
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		if results == nil {
			results = []pluginSeedResult{}
		}
		return output.EmitData(pilotWriter(), "db seed", pluginSeedData{Plugins: results})
	}
	for _, r := range results {
		fmt.Printf("  %s: %s\n", r.Name, r.Result)
	}
	return nil
}

// installedPluginNames lists plugin directory names that hold a plugin.json.
func installedPluginNames(pluginDir string) ([]string, error) {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errs.Wrap("E100", "reading the plugin directory "+pluginDir, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || !pluginSeedNameRe.MatchString(e.Name()) {
			continue
		}
		if _, serr := os.Stat(filepath.Join(pluginDir, e.Name(), "plugin.json")); serr == nil {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// seedOnePlugin loads the plugin's manifest and runs its seed. With
// skipMissing, a plugin without seed.command is reported as skipped.
func seedOnePlugin(ctx context.Context, rt seed.PluginRuntime, pluginDir, name string, skipMissing bool) (pluginSeedResult, error) {
	res := pluginSeedResult{Name: name}
	if !pluginSeedNameRe.MatchString(name) {
		return res, errs.Newf("E401", "invalid plugin name %q", name)
	}
	root := filepath.Join(pluginDir, name)
	m, err := loadSeedManifest(root, name)
	if err != nil {
		return res, err
	}
	if m.Seed == nil || len(m.Seed.Command) == 0 {
		if skipMissing {
			res.Result = "skipped"
			return res, nil
		}
		return res, seed.ValidatePluginArgv(name, nil)
	}
	fragment := pluginComposeDefault
	if m.Service != nil && m.Service.Compose != nil && *m.Service.Compose != "" {
		fragment = *m.Service.Compose
	}
	if filepath.IsAbs(fragment) || strings.HasPrefix(filepath.Clean(fragment), "..") {
		return res, errs.Newf("E106", "plugin %q service.compose %q leaves the plugin directory", name, fragment)
	}
	svc, err := seed.FirstComposeService(filepath.Join(root, filepath.Clean(fragment)))
	if err != nil {
		return res, err
	}
	if _, err := seed.RunPluginSeed(ctx, rt, seed.PluginTarget{Name: name, Service: svc, Argv: m.Seed.Command}); err != nil {
		return res, err
	}
	res.Result = "seeded"
	return res, nil
}

func loadSeedManifest(root, name string) (*manifestv2.Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, "plugin.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errs.Newf("E100", "plugin %q is not installed", name).
				WithFix("Install it with `nself add " + name + "`.")
		}
		return nil, errs.Wrap("E106", "reading the manifest of plugin "+name, err)
	}
	m, err := manifestv2.ParseQuiet(data)
	if err != nil {
		return nil, errs.Wrap("E106", "plugin "+name+" manifest is invalid: "+err.Error(), err)
	}
	return m, nil
}
