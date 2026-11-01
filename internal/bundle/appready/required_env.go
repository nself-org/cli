package appready

// Purpose: fail bundle installation before starting containers with missing inputs.
// Inputs: installed manifest declarations and injected Compose interpolation values.
// Outputs: a plugin-scoped error naming missing variables, never their values.
// Constraints: config parsing stays with the caller's configuration helpers.

import (
	"context"
	"fmt"
	"strings"
)

// RequiredEnvPreflight checks required inputs without declared fallback values.
func RequiredEnvPreflight(load func() (map[string]string, error)) func(context.Context, string, []string) error {
	return func(_ context.Context, pluginDir string, names []string) error {
		values, err := load()
		if err != nil {
			return err
		}
		members, err := LoadMembers(pluginDir, names)
		if err != nil {
			return err
		}
		for _, member := range members {
			var missing []string
			for _, variable := range member.EnvVars {
				if variable.Required && variable.Name != "" && variable.Default == "" && strings.TrimSpace(values[variable.Name]) == "" {
					missing = append(missing, variable.Name)
				}
			}
			if len(missing) > 0 {
				return &StepError{Plugin: member.Name, Err: fmt.Errorf("plugin %s requires %s in the project .env; set the missing variables and retry bundle install", member.Name, strings.Join(missing, ", "))}
			}
		}
		return nil
	}
}
