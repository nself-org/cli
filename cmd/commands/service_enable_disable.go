package commands

// Purpose: Implements `nself service enable <name>` and `nself service
// disable <name>`, which flip a service's *_ENABLED env var and update the
// docker-compose profile set accordingly. Split out of service.go
// (CLI-R12) to separate these two handlers from the cobra command
// definitions and the other add/upgrade/list/configure/lifecycle handlers
// in the other service_*.go files.
// Inputs: the cobra.Command + args (service name) and the --env flag value.
// Outputs: an updated env file entry and printed confirmation.
// Constraints: pure move — no behavior changes. Both handlers keep calling
// canonicalServiceName (service_add_upgrade.go) to resolve aliases.

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/spf13/cobra"
)

func runServiceEnable(cmd *cobra.Command, args []string) error {
	envFlag, _ := cmd.Flags().GetString("env")

	canonName, svc, err := canonicalServiceName(args[0])
	if err != nil {
		return err
	}

	envFile, err := resolveEnvFile(envFlag)
	if err != nil {
		return err
	}

	// Check current state — idempotency guard.
	values, err := readEnvValues(envFile)
	if err != nil {
		return err
	}
	if v, ok := values[svc.EnvVar]; ok && strings.ToLower(strings.TrimSpace(v)) == "true" {
		fmt.Printf("%s is already enabled\n", canonName)
		return nil
	}

	if err := setEnvKeyInFile(envFile, svc.EnvVar, "true"); err != nil {
		return fmt.Errorf("enabling %s: %w", canonName, err)
	}

	if canonName == "minio" && compat.V15() {
		// compat.V15(D-0683): core MinIO -> plugin; point at the core store toggle
		fmt.Fprintln(os.Stderr, "Note: in v1.5 MinIO is the minio plugin (run `nself add minio`). For the core object store run `nself service enable object-storage`.")
	}
	fmt.Printf("%s enabled. Run `nself build` to apply changes.\n", canonName)
	return nil
}

func runServiceDisable(cmd *cobra.Command, args []string) error {
	envFlag, _ := cmd.Flags().GetString("env")

	canonName, svc, err := canonicalServiceName(args[0])
	if err != nil {
		return err
	}

	envFile, err := resolveEnvFile(envFlag)
	if err != nil {
		return err
	}

	// Dependency warning: check if any enabled services depend on this one.
	if deps, ok := serviceDependents[canonName]; ok {
		values, _ := readEnvValues(envFile)
		var activeDeps []string
		for _, dep := range deps {
			for _, depSvc := range optionalServices() {
				if depSvc.Name == dep {
					if v, ok := values[depSvc.EnvVar]; ok && strings.ToLower(strings.TrimSpace(v)) == "true" {
						activeDeps = append(activeDeps, dep)
					}
				}
			}
		}
		if len(activeDeps) > 0 {
			fmt.Fprintf(os.Stderr, "Warning: the following services depend on %s: %s\n", canonName, strings.Join(activeDeps, ", "))
			fmt.Fprint(os.Stderr, "Continue? [y/N] ")
			var response string
			_, _ = fmt.Scanln(&response)
			if strings.ToLower(strings.TrimSpace(response)) != "y" {
				fmt.Fprintln(os.Stderr, "Aborted.")
				return nil
			}
		}
	}

	if err := setEnvKeyInFile(envFile, svc.EnvVar, "false"); err != nil {
		return fmt.Errorf("disabling %s: %w", canonName, err)
	}

	fmt.Printf("%s disabled. Run `nself build` to apply changes.\n", canonName)
	return nil
}
