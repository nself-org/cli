package commands

// Purpose: Implements `nself service add <name>` and `nself service upgrade
// <name>`, plus their shared helpers — resolving a service's canonical name
// and env-file location, listing upgradeable service names, and reading/
// writing individual keys in an env file. Split out of service.go (CLI-R12)
// to separate the add/upgrade command bodies from the cobra command
// definitions and the list/enable/disable/configure/lifecycle handlers that
// live in the other service_*.go files.
// Inputs: the cobra.Command + args for add/upgrade, an env-file path or
// --env flag value, and (for canonicalServiceName) a raw service name/alias.
// Outputs: scaffolded service files (add), an updated version env var
// (upgrade), and the resolved serviceEntry / env-file map used by callers.
// Constraints: pure move — no behavior changes. canonicalServiceName,
// readEnvValues, and setEnvKeyInFile are also called from service.go's
// remaining command wiring and from other service_*.go files in this
// split — their signatures and behavior are unchanged.

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/nself-org/cli/internal/scaffold"
	"github.com/nself-org/cli/internal/security"
	"github.com/nself-org/cli/internal/service"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// runServiceAdd implements 'nself service add <name>'.
func runServiceAdd(cmd *cobra.Command, args []string) error {
	name := strings.ToLower(strings.TrimSpace(args[0]))
	// --template is the canonical flag; --lang is a hidden backward-compat alias.
	lang, _ := cmd.Flags().GetString("template")
	if legacyLang, _ := cmd.Flags().GetString("lang"); legacyLang != "" {
		lang = legacyLang
	}
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if name == "" {
		return fmt.Errorf("service name must not be empty")
	}

	// Validate name: reuse the same rules as CS_N parsing (lowercase alphanumeric + hyphens/underscores, 2-63 chars).
	csNameRe := regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,61}[a-z0-9]$`)
	if !csNameRe.MatchString(name) {
		return fmt.Errorf("invalid service name %q: must be lowercase alphanumeric with hyphens/underscores, 2-63 chars", name)
	}

	if !scaffold.IsValidLang(lang) {
		return fmt.Errorf("unsupported language %q; choose one of: %s",
			lang, strings.Join(scaffold.SupportedLangs(), ", "))
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	opts := scaffold.Options{
		ImageRef:   compose.ImageRef,
		Name:       name,
		Lang:       lang,
		ProjectDir: cwd,
		Force:      force,
		DryRun:     dryRun,
	}

	result, err := scaffold.Run(opts)
	if err != nil {
		return err
	}

	if dryRun {
		ui.Info(fmt.Sprintf("Dry run — no files written. Would scaffold service %q (%s):", name, lang))
		fmt.Printf("  Slot:     %s (port %d)\n", result.EnvKey, 8000+result.Slot)
		fmt.Printf("  Env:      %s=%s\n", result.EnvKey, result.EnvValue)
		fmt.Printf("  EnvFile:  %s\n", result.EnvFile)
		fmt.Printf("  Dir:      %s\n", result.ServiceDir)
		fmt.Printf("  Files:\n")
		for _, f := range result.Files {
			fmt.Printf("    %s\n", f)
		}
		return nil
	}

	ui.Success(fmt.Sprintf("Custom service %q scaffolded.", name))
	fmt.Printf("  Slot:    %s = %s\n", result.EnvKey, result.EnvValue)
	fmt.Printf("  Dir:     %s\n", result.ServiceDir)
	fmt.Printf("  EnvFile: %s (updated)\n", result.EnvFile)
	fmt.Println()
	fmt.Printf("Next steps:\n")
	fmt.Printf("  Edit %s and implement your service\n", result.ServiceDir)
	fmt.Printf("  Run 'nself build' to regenerate docker-compose.yml\n")
	fmt.Printf("  Run 'nself start' to launch the full stack\n")
	return nil
}

// runServiceUpgrade writes <NAME>_VERSION=<ver> into the .env file.
func runServiceUpgrade(cmd *cobra.Command, args []string) error {
	name := strings.ToLower(strings.TrimSpace(args[0]))
	version := strings.TrimSpace(args[1])

	// Resolve aliases (e.g. "mailpit" → "email", "meilisearch" → "search").
	if canonical, ok := serviceAliases[name]; ok {
		name = canonical
	}

	// Validate: "mailpit" also maps to email via reverse lookup in knownServices.
	// For core services (postgres, hasura, auth, nginx) accept them directly.
	envKey, knownVersion := serviceVersionKeys[name]
	if !knownVersion {
		return fmt.Errorf("unknown service %q; upgradeable services: %s",
			name, strings.Join(upgradeableServiceNames(), ", "))
	}

	// Sanitize version string: alphanumeric plus ., -, _ only.
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("invalid version string %q: only alphanumeric, dots, hyphens, and underscores are allowed", version)
	}
	if len(version) > 128 {
		return fmt.Errorf("version string too long (max 128 chars)")
	}

	envFlag, _ := cmd.Flags().GetString("env")
	envFile, err := resolveEnvFile(envFlag)
	if err != nil {
		return err
	}

	if err := setEnvKeyInFile(envFile, envKey, version); err != nil {
		return fmt.Errorf("setting version for %s: %w", name, err)
	}

	fmt.Printf("%s version pinned to %s (%s=%s)\n", name, version, envKey, version)
	fmt.Println("Run `nself build` to apply the change.")
	return nil
}

// upgradeableServiceNames returns a sorted list of services that support version pinning.
func upgradeableServiceNames() []string {
	names := make([]string, 0, len(serviceVersionKeys))
	for k := range serviceVersionKeys {
		names = append(names, k)
	}
	return names
}

// --- helpers ---

// resolveEnvFile returns the .env filename for the given --env flag value.
// An empty env string resolves to ".env" in the current working directory.
func resolveEnvFile(envFlag string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	filename := ".env"
	if envFlag != "" {
		filename = ".env." + envFlag
	}
	return cwd + "/" + filename, nil
}

// mlflowRedirectMsg is returned when a user tries to enable/disable mlflow via
// the service command after the v1.1.0 reclassification.
const mlflowRedirectMsg = "mlflow is now a plugin: run `nself plugin install mlflow`\n" +
	"To uninstall: `nself plugin uninstall mlflow`"

// canonicalServiceName resolves aliases and validates the service name.
// Returns the canonical name and the serviceEntry, or an error if unknown.
// Returns a special errMLflowRedirect sentinel for the mlflow redirect case.
func canonicalServiceName(input string) (string, serviceEntry, error) {
	lower := strings.ToLower(strings.TrimSpace(input))

	// MLflow redirect: was an optional service, now a free plugin.
	if lower == "mlflow" {
		return "", serviceEntry{}, fmt.Errorf("%s", mlflowRedirectMsg)
	}

	// "storage" is the free storage plugin once the stack declares it (D-0451):
	// the env toggles below only manage core MinIO, so refuse rather than flip
	// MINIO_ENABLED for a plugin the operator meant.
	if lower == "storage" {
		if dir, err := os.Getwd(); err == nil && service.ComposeDeclaresService(dir, "storage") {
			return "", serviceEntry{}, fmt.Errorf("%q is the storage plugin in this project, not MinIO; manage it with `nself plugin install|remove storage`, or use `minio` for the core MinIO toggle", input)
		}
	}

	// Resolve alias to canonical name.
	if canonical, ok := serviceAliases[lower]; ok {
		lower = canonical
	}

	for _, svc := range knownServices {
		if svc.Name == lower {
			return lower, svc, nil
		}
	}

	// Build list of valid names including aliases.
	validNames := make([]string, 0, len(knownServices)+len(serviceAliases))
	for _, svc := range knownServices {
		validNames = append(validNames, svc.Name)
	}
	for alias := range serviceAliases {
		validNames = append(validNames, alias)
	}
	return "", serviceEntry{}, fmt.Errorf("unknown service %q\nValid names: %s", input, strings.Join(validNames, ", "))
}

// readEnvValues reads key=value pairs from the given file.
// Returns an empty map (not an error) if the file does not exist.
func readEnvValues(filename string) (map[string]string, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	values, err := config.ReadEnvFile(filename)
	if err != nil {
		return nil, config.EnvReadError(filename, err)
	}
	return values, nil
}

// setEnvKeyInFile sets KEY=value in filename and writes it back (0600), using
// reconcile.EnvFileWithLine, the encoder `config set` uses: comments and line
// order are kept and the value is stored so the loader reads it back unchanged
// ('$' is escaped, never expanded). Safe for files that do not yet exist.
func setEnvKeyInFile(filename, key, value string) error {
	old, readErr := os.ReadFile(filename)
	exists := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("reading %s: %w", filename, readErr)
	}
	content, _, encErr := reconcile.EnvFileWithLine(old, exists, key, value)
	if encErr != nil {
		return fmt.Errorf("encoding %s for %s: %w", key, filename, encErr)
	}
	// Env files must be 0600 (user-readable only).
	if err := os.WriteFile(filename, content, 0600); err != nil {
		return fmt.Errorf("writing %s: %w", filename, err)
	}
	if err := security.EnforceFilePermissions(filename, 0600); err != nil {
		return fmt.Errorf("enforcing permissions on %s: %w", filename, err)
	}
	return nil
}
