package commands

// Builtin extension toggles and the post-add command hint.
// Inputs: a single builtin slug or successfully installed plugin names.
// Outputs: the shared disable marker and one confirmation or Next line.
// Constraints: unknown slugs keep the existing plugin/bundle dispatch.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

func toggleBuiltin(args []string, add bool) (bool, error) {
	if len(args) != 1 {
		return false, nil
	}
	for _, family := range builtinFamilies {
		if family.slug != args[0] {
			continue
		}
		path := filepath.Join(resolvePluginDir(), family.slug, ".disabled")
		if add {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return true, fmt.Errorf("enable builtin %q: %w", family.slug, err)
			}
			fmt.Printf("Builtin %q enabled.\n", family.slug)
			return true, nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return true, fmt.Errorf("prepare builtin %q: %w", family.slug, err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return true, fmt.Errorf("disable builtin %q: %w", family.slug, err)
		}
		fmt.Printf("Builtin %q disabled.\n", family.slug)
		return true, nil
	}
	return false, nil
}

func printAddedCommandHints(names []string) {
	// compat.V15(P7-CANON-08): no hint -> Next: nself <command> --help
	if !compat.V15() {
		return
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(resolvePluginDir(), name, "plugin.json"))
		if err != nil {
			continue
		}
		manifest, err := manifestv2.ParseQuiet(data)
		if err == nil && manifest.Commands != nil && manifest.Commands.Command != "" {
			fmt.Printf("Next: nself %s --help\n", manifest.Commands.Command)
		}
	}
}
