package commands

import (
	"regexp"

	"github.com/spf13/cobra"
)

// versionPattern validates service version strings: semver, tags like "latest",
// "alpine", "16.3", "v2.40.0", and simple numeric versions.
var versionPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._\-]*$`)

// serviceEntry describes a single optional service and its governing env var.
type serviceEntry struct {
	Name   string
	EnvVar string
	Port   int // 0 means N/A (or "multi" for monitoring)
}

// knownServices is the ordered list of optional services managed by `nself service`.
// MLflow was reclassified to a free plugin in v1.1.0 — use `nself plugin install mlflow`.
var knownServices = []serviceEntry{
	{Name: "redis", EnvVar: "REDIS_ENABLED", Port: 6379},
	{Name: "minio", EnvVar: "MINIO_ENABLED", Port: 9000},
	{Name: "email", EnvVar: "MAILPIT_ENABLED", Port: 8025},
	{Name: "functions", EnvVar: "FUNCTIONS_ENABLED", Port: 3008},
	{Name: "search", EnvVar: "SEARCH_ENABLED", Port: 7700},
	{Name: "monitoring", EnvVar: "MONITORING_ENABLED", Port: 0},
	{Name: "admin", EnvVar: "NSELF_ADMIN_ENABLED", Port: 3021},
}

// serviceAliases maps alternate names to canonical service names.
var serviceAliases = map[string]string{
	"storage":     "minio",
	"mail":        "email",
	"mailpit":     "email",
	"meilisearch": "search",
}

// serviceDependents maps a service name to the services that depend on it.
var serviceDependents = map[string][]string{
	"redis":  {"functions", "notify", "cron"},
	"minio":  {"storage"},
	"search": {"cms"},
}

// serviceCmd is the parent command for optional service management.
var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage optional services",
	Long: `Enable, disable, and list optional nSelf services (6 total).

Available services:
  redis        Cache and queue (REDIS_ENABLED)
  minio        S3-compatible storage (MINIO_ENABLED), alias: storage
  email        Email testing via Mailpit (MAILPIT_ENABLED), aliases: mail, mailpit
  functions    Serverless runtime (FUNCTIONS_ENABLED)
  search       Full-text search via MeiliSearch (SEARCH_ENABLED), alias: meilisearch
  monitoring   Prometheus/Grafana stack (MONITORING_ENABLED)
  admin        nSelf Admin GUI (NSELF_ADMIN_ENABLED)

MLflow is now a free plugin: run 'nself plugin install mlflow'

After enabling or disabling a service, run 'nself build' to apply changes.`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself service` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var serviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all optional services with enabled/disabled status",
	Long: `List optional services and whether this project has them enabled.

With --core, list the nSelf service catalog instead: which services every stack
requires, which are opt-in, the environment variable that enables each one, and
the pinned default image. This reads the compose catalog, not your .env, so it
works outside a project directory.

Examples:
  nself service list
  nself service list --core
  nself service list --core --json`,
	RunE: runServiceList,
}

var serviceEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable an optional service",
	Args:  cobra.ExactArgs(1),
	RunE:  runServiceEnable,
}

var serviceDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable an optional service",
	Args:  cobra.ExactArgs(1),
	RunE:  runServiceDisable,
}

// serviceVersionKeys maps a canonical service name to its .env version key.
var serviceVersionKeys = map[string]string{
	"postgres":  "POSTGRES_VERSION",
	"hasura":    "HASURA_VERSION",
	"auth":      "AUTH_VERSION",
	"nginx":     "NGINX_VERSION",
	"redis":     "REDIS_VERSION",
	"minio":     "MINIO_VERSION",
	"email":     "MAILPIT_VERSION",
	"functions": "FUNCTIONS_VERSION",
	"search":    "MEILISEARCH_VERSION",
	"admin":     "NSELF_ADMIN_VERSION",
}

var serviceUpgradeCmd = &cobra.Command{
	Use:   "upgrade <name> <version>",
	Short: "Pin a service to a specific image version",
	Long: `Write the version tag for the named service into your .env file and prompt
you to run 'nself build' to apply the change.

Examples:
  nself service upgrade postgres 16.3
  nself service upgrade hasura v2.40.0
  nself service upgrade auth latest`,
	Args: cobra.ExactArgs(2),
	RunE: runServiceUpgrade,
}

// serviceStartCmd starts a named nSelf service via `docker compose up -d --no-deps`.
var serviceStartCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a named nSelf service",
	Long: `Start a named nSelf service using the existing docker-compose.yml.

The service must already be configured in the stack (run 'nself build' first).
This command is equivalent to 'docker compose up -d --no-deps <service>'.

Examples:
  nself service start redis
  nself service start minio
  nself service start search`,
	Args: cobra.ExactArgs(1),
	RunE: runServiceStart,
}

// serviceStopCmd stops a named nSelf service (container preserved).
var serviceStopCmd = &cobra.Command{
	Use:   "stop <name>",
	Short: "Stop a named nSelf service (container preserved)",
	Long: `Stop a named nSelf service without removing its container.

The container state is preserved so 'nself service start <name>' can resume it
without re-creating volumes or losing data.

Examples:
  nself service stop redis
  nself service stop minio`,
	Args: cobra.ExactArgs(1),
	RunE: runServiceStop,
}

// serviceRestartCmd restarts a named nSelf service.
var serviceRestartCmd = &cobra.Command{
	Use:   "restart <name>",
	Short: "Restart a named nSelf service",
	Long: `Restart a running nSelf service. Equivalent to 'docker compose restart <service>'.

Examples:
  nself service restart redis
  nself service restart hasura`,
	Args: cobra.ExactArgs(1),
	RunE: runServiceRestart,
}

// servicePsCmd shows current status of all nSelf stack services.
var servicePsCmd = &cobra.Command{
	Use:   "ps",
	Short: "Show status of all nSelf stack services",
	Long: `Show the current status of every service in the running nSelf stack.

Reads live container state from 'docker compose ps'. Run 'nself start' first
if no services appear.`,
	RunE: runServicePs,
}

// serviceUpdateCmd pulls the latest image for a service and restarts it.
var serviceUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Pull the latest image for a service and restart it",
	Long: `Pull the latest image for a named service and restart it.

Equivalent to:
  docker compose pull <service>
  docker compose up -d --no-deps <service>

Examples:
  nself service update redis
  nself service update hasura`,
	Args: cobra.ExactArgs(1),
	RunE: runServiceUpdate,
}

// serviceScaleCmd adjusts the replica count for a named service.
var serviceScaleCmd = &cobra.Command{
	Use:   "scale <name> <replicas>",
	Short: "Set the replica count for a named service",
	Long: `Set the number of replicas for a named nSelf service.

Replicas must be at least 1. This wraps 'docker compose up -d --scale <service>=<n>'.

Examples:
  nself service scale functions 3
  nself service scale redis 1`,
	Args: cobra.ExactArgs(2),
	RunE: runServiceScale,
}

// serviceAddCmd scaffolds a new CS_N custom service into the current nSelf project.
var serviceAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Scaffold a custom service (CS_N slot) into the current project",
	Long: `Scaffold a new custom service into the current nSelf project.

The command:
  1. Finds the next available CS_N slot (1-10) in .env.dev
  2. Creates a services/<name>/ directory with starter files
  3. Writes CS_N=<name>:<template>:<port> and <NAME>_PORT=<port> into .env.dev

Run 'nself build' after adding a service to regenerate docker-compose.yml.

Supported templates: go (default), node, python, static, rust, other

Examples:
  nself service add myapi
  nself service add myapi --template python
  nself service add myapi --template node --dry-run
  nself service add mysite --template static`,
	Args: cobra.ExactArgs(1),
	RunE: runServiceAdd,
}

func init() {
	serviceCmd.PersistentFlags().String("env", "", "Target environment (reads .env.{env})")
	serviceListCmd.Flags().Bool("json", false, "Output as JSON array")
	serviceListCmd.Flags().Bool("core", false, "List the service catalog (required vs optional) instead of this project's status")

	// service add flags
	serviceAddCmd.Flags().String("template", "go", "Service template: go, node, python, static, rust, other")
	serviceAddCmd.Flags().String("lang", "", "Alias for --template (deprecated, use --template)")
	_ = serviceAddCmd.Flags().MarkHidden("lang")
	serviceAddCmd.Flags().Bool("force", false, "Overwrite existing service directory")
	serviceAddCmd.Flags().Bool("dry-run", false, "Print what would be done without writing files")

	serviceCmd.AddCommand(serviceListCmd)
	serviceCmd.AddCommand(serviceEnableCmd)
	serviceCmd.AddCommand(serviceDisableCmd)
	serviceCmd.AddCommand(serviceUpgradeCmd)
	serviceCmd.AddCommand(serviceConfigureCmd)
	serviceCmd.AddCommand(serviceAddCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceRestartCmd)
	serviceCmd.AddCommand(servicePsCmd)
	serviceCmd.AddCommand(serviceUpdateCmd)
	serviceCmd.AddCommand(serviceScaleCmd)

	RootCmd.AddCommand(serviceCmd)
}
