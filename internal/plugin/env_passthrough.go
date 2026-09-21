package plugin

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"

	"github.com/nself-org/cli/internal/config"
)

// Handing a command plugin the project configuration it declares.
//
// Purpose: a service plugin runs in a container whose environment compose has
// already populated. A CLI plugin runs on the user's machine, where nothing has
// read the .env cascade, so os.Getenv returns nothing for every project
// setting. Without this, a command extracted under CLI-R11 either loses access
// to project configuration or re-implements the cascade — and the cascade
// having exactly one implementation is what CLI-R18 was for.
//
// Inputs: the project directory, and the plugin's manifest.
//
// Outputs: environment entries to add to the plugin process, resolved through
// the same cascade order the CLI itself uses.
//
// Constraints: only variables the manifest DECLARES are passed. The CLI's own
// process environment is never modified — config.Load uses godotenv.Overload,
// which would export every value in .env.secrets to the CLI and then to every
// child it spawns. Reading the files directly keeps the blast radius to what a
// plugin asked for, which is visible in its manifest at install time.
//
// This is not a security boundary. A CLI plugin runs as the user and can read
// .env.secrets off disk. The point is that what a plugin receives is declared
// and auditable rather than implicit.

// PluginEnv returns the environment entries to append for a plugin process.
//
// Returns nil when the plugin declares nothing, when there is no project here,
// or when a variable is not set anywhere — an absent value is passed as absent,
// not as empty, so the plugin's own default applies.
func PluginEnv(projectDir string, m *PluginManifest) []string {
	if m == nil || len(m.EnvVars) == 0 {
		return nil
	}

	resolved := resolveCascade(projectDir)
	if len(resolved) == 0 {
		return nil
	}

	out := make([]string, 0, len(m.EnvVars))
	for _, v := range m.EnvVars {
		if v.Name == "" {
			continue
		}
		// A value already in the environment wins: the user typed it on this
		// invocation, or their shell exports it deliberately. This is the
		// opposite of godotenv.Overload, which the CLI uses internally, and it
		// is the right way round for a child process — an explicit
		// FOO=bar nself <cmd> must reach the plugin.
		if _, ok := os.LookupEnv(v.Name); ok {
			continue
		}
		if val, ok := resolved[v.Name]; ok {
			out = append(out, v.Name+"="+val)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// resolveCascade reads the project's .env files in the canonical order and
// returns the merged result.
//
// The order comes from config.EnvCascadeOrder, which is the single source of
// truth CLI-R18 established and which `nself env explain` and the migration
// shim also read. Duplicating the order here instead would recreate exactly the
// drift that rule exists to prevent.
//
// envName itself comes from config.ResolveEnv, the same resolver config.Load
// and `nself env explain` use (process environment wins; otherwise the
// project's own .env's ENV= key; otherwise "dev"). A bare os.Getenv("ENV")
// here would default to "dev" on a production box whose only signal is
// .env's ENV=prod — a plugin-invoking command would then read .env.dev
// while `nself build` reads .env.prod for the same project.
//
// godotenv.Read is used rather than Load or Overload because those mutate the
// process environment. This function must not: the CLI's own environment stays
// as the user left it, and only declared values reach the child.
func resolveCascade(projectDir string) map[string]string {
	if projectDir == "" {
		projectDir = "."
	}
	if !hasEnvCascade(projectDir) {
		return nil
	}

	envName, _ := config.ResolveEnv(projectDir)

	merged := make(map[string]string)
	for _, name := range config.EnvCascadeOrder(envName, config.LegacyOrderActive()) {
		path := filepath.Join(projectDir, name)
		vals, err := godotenv.Read(path)
		if err != nil {
			continue // absent or unreadable: the cascade skips it, as Load does
		}
		// Later files win, which is what "cascade" means here.
		for k, v := range vals {
			merged[k] = v
		}
	}
	return merged
}

// hasEnvCascade reports whether projectDir looks like an nself project, so that
// running a plugin outside one does no filesystem work and passes nothing.
func hasEnvCascade(projectDir string) bool {
	for _, name := range config.AllCascadeFilenames {
		if _, err := os.Stat(filepath.Join(projectDir, name)); err == nil {
			return true
		}
	}
	return false
}
