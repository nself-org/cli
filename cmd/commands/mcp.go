package commands

// mcp.go — the nSelf MCP server command and its tool registrations.
//
// Purpose: CLI-R15 "MCP v2". Exposes the real core CLI surface (status,
//   doctor, urls, logs, service list, env list, config get/set/show, build,
//   start/stop/restart, db migrate status, backup list, deploy status,
//   plugin list/install) plus the pre-existing Hasura data tools, as MCP
//   tools with output schemas and annotations, so an agent can plan around
//   them instead of guessing from prose.
// Inputs:  --transport (stdio|sse|http), --port, --host, --allowed-host, --timeout,
//   NSELF_MCP_TOKEN or ~/.nself/mcp-token (bearer auth on sse/http — see
//   mcp_auth.go for the bind and token policy).
// Outputs: an MCP server over the chosen transport.
// Constraints: every mcp.NewTool(...) registration call for the non-Sentry
//   tools MUST stay physically in this file (or mcp_sentry.go) —
//   tools/parity/mcptools.go hardcodes cmd/commands/mcp.go and
//   cmd/commands/mcp_sentry.go as the only two files it text-scans for tool
//   coverage, and this ticket may not edit tools/parity/**. Handler
//   implementations live in mcp_tools_core.go / mcp_tools_mutate.go /
//   mcp_tools_data.go to keep this file under the 300-line cap; only the
//   registration call sites are here.
// SPORT: CLI-CMD-MCP-001

import (
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/spf13/cobra"
)

const mcpServerPort = 3825

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the nSelf MCP server",
	Long: `Start a Model Context Protocol (MCP) server exposing nSelf as tools,
resources, and prompts for Claude Code and other MCP clients.

Runs on stdio by default — the correct mode for Claude Code's mcpServers
config. Use --transport sse or --transport http to expose the server over
a local HTTP port instead. The HTTP transports require a bearer token: set
NSELF_MCP_TOKEN, or keep one in ~/.nself/mcp-token (mode 0600). With neither,
the first start generates the file and prints its path, never the token.
(v1.4 mode, the current default, serves an open loopback port with a warning
when no token is set.)

--host sets the bind address (default 127.0.0.1). A non-loopback host needs
--allowed-host <name> (repeatable) plus a configured token on every start, and
prints a warning that the token crosses the network over cleartext HTTP.

Run 'nself mcp' from inside an nSelf project directory (elsewhere it serves
init and the read-only tools until one exists). --timeout bounds each call.
See the generated tool list at .github/wiki/MCP-Tools.md and the resources and
prompts at .github/wiki/cmd-mcp.md.

Claude Code config (.claude/settings.json):
  "mcpServers": {
    "nself": {
      "command": "nself",
      "args": ["mcp"]
    }
  }`,
	Args: cobra.NoArgs,
	RunE: runMCPServe,
}

func init() {
	mcpCmd.Flags().StringP("transport", "t", "stdio", "Transport: stdio, sse, or http")
	mcpCmd.Flags().IntP("port", "p", mcpServerPort, "Port for the sse/http transports")
	mcpCmd.Flags().String("host", "127.0.0.1", "Bind address for the sse/http transports")
	mcpCmd.Flags().StringSlice("allowed-host", nil, "Host name this server is reachable under; required (repeatable) for a non-loopback --host")
	mcpCmd.Flags().Duration("timeout", invoke.DefaultTimeout, "Longest one tool call may run before its command is stopped, such as 15m")
	RootCmd.AddCommand(mcpCmd)
}

func runMCPServe(cmd *cobra.Command, args []string) error {
	transport, _ := cmd.Flags().GetString("transport")
	port, _ := cmd.Flags().GetInt("port")
	host, _ := cmd.Flags().GetString("host")
	allowed, _ := cmd.Flags().GetStringSlice("allowed-host")

	timeout, _ := cmd.Flags().GetDuration("timeout")
	if timeout < time.Second {
		return errs.Newf("E401", "--timeout %s is too short", timeout).WithFix("Pass a duration of at least 1s, such as 15m.")
	}

	// Bind and token policy first: a refused start must not build anything or
	// write a token file. stdio skips it.
	wrap, err := prepareMCPHTTP(transport, host, allowed, os.Stderr)
	if err != nil {
		return err
	}

	// compat.V15(P7-SURF-03): 26 hand tools primary -> generated tools, merged same-name tools, deprecated aliases
	s, err := newMCPServer(transport, compat.V15(), timeout)
	if err != nil {
		return err
	}

	return serveMCP(s, transport, host, port, wrap)
}

// registerMCPTools attaches every MCP tool: ɳSentry (mcp_sentry.go), the
// core CLI surface (this file), and the Hasura data tools (this file).
func registerMCPTools(s *server.MCPServer) {
	registerSentryMCPTools(s)
	registerCoreMCPTools(s)
	registerDataMCPTools(s)
}

// registerCoreMCPTools registers every tool backed by a `nself` command
// (status/doctor/urls/logs/service/env/config/build/start/stop/restart/
// db/backup/deploy/plugin) — see mcp_tools_core.go and mcp_tools_mutate.go
// for the handlers.
func registerCoreMCPTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("nself_status",
		mcp.WithDescription("Health status of every service in the current nSelf project"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOutputSchema[health.HealthReport](),
	), mcpStatusHandler())

	s.AddTool(mcp.NewTool("nself_doctor",
		mcp.WithDescription("Run the full nSelf diagnostic suite (Docker, disk, ports, certs, and more)"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOutputSchema[DoctorResult](),
	), mcpDoctorHandler())

	s.AddTool(mcp.NewTool("nself_urls",
		mcp.WithDescription("Computed service URLs for the current project, grouped by required/optional/custom/frontend"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOutputSchema[urlsOutput](),
	), mcpURLsHandler())

	s.AddTool(mcp.NewTool("nself_logs",
		mcp.WithDescription("Tail recent docker compose logs for one service, or the whole stack"),
		mcp.WithString("service", mcp.Description("Service name (empty = all services)")),
		mcp.WithNumber("lines", mcp.Description("Number of lines to return (default 50)")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOutputSchema[LogsResult](),
	), mcpLogsHandler())

	s.AddTool(mcp.NewTool("nself_service_list",
		mcp.WithDescription("List running/stopped services with container name, status, and health"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOutputSchema[ServiceListResult](),
	), mcpServiceListHandler())

	s.AddTool(mcp.NewTool("nself_env_list",
		mcp.WithDescription("List available environments (dev/staging/prod) and which is active"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOutputSchema[EnvListResult](),
	), mcpEnvListHandler())

	s.AddTool(mcp.NewTool("nself_config_get",
		mcp.WithDescription("Read one config key from the project's env file. Secret values are always redacted."),
		mcp.WithString("key", mcp.Required(), mcp.Description("Config key, e.g. BASE_DOMAIN")),
		mcp.WithString("env", mcp.Description("Env suffix (dev/staging/prod); empty = plain .env")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOutputSchema[ConfigGetResult](),
	), mcpConfigGetHandler())

	s.AddTool(mcp.NewTool("nself_config_show",
		mcp.WithDescription("List every config key/value in the project's env file. Secret values are always redacted."),
		mcp.WithString("env", mcp.Description("Env suffix (dev/staging/prod); empty = plain .env")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOutputSchema[ConfigShowResult](),
	), mcpConfigShowHandler())

	s.AddTool(mcp.NewTool("nself_config_set",
		mcp.WithDescription("Set one config key in the project's env file"),
		mcp.WithString("key", mcp.Required(), mcp.Description("Config key, uppercase letters/digits/underscore only")),
		mcp.WithString("value", mcp.Required(), mcp.Description("Value to set")),
		mcp.WithString("env", mcp.Description("Env suffix (dev/staging/prod); empty = plain .env")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOutputSchema[ConfigSetResult](),
	), mcpConfigSetHandler())

	s.AddTool(mcp.NewTool("nself_build",
		mcp.WithDescription("Regenerate docker-compose.yml and nginx config from the project's env files. Overwrites prior generated output."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOutputSchema[ExecResult](),
	), mcpLifecycleHandler("build", "--quiet"))

	s.AddTool(mcp.NewTool("nself_start",
		mcp.WithDescription("Start the nSelf stack (docker compose up). Safe to call when already running."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOutputSchema[ExecResult](),
	), mcpLifecycleHandler("start"))

	s.AddTool(mcp.NewTool("nself_stop",
		mcp.WithDescription("Stop the nSelf stack. Causes downtime for any consumer of this project's services."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOutputSchema[ExecResult](),
	), mcpLifecycleHandler("stop"))

	s.AddTool(mcp.NewTool("nself_restart",
		mcp.WithDescription("Restart the nSelf stack. Causes brief downtime."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOutputSchema[ExecResult](),
	), mcpLifecycleHandler("restart"))

	s.AddTool(mcp.NewTool("nself_db_migrate_status",
		mcp.WithDescription("List database migrations and whether each has been applied"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOutputSchema[MigrateStatusResult](),
	), mcpDBMigrateStatusHandler())

	s.AddTool(mcp.NewTool("nself_backup_list",
		mcp.WithDescription("List local backups with id, date, size, and type"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOutputSchema[BackupListResult](),
	), mcpBackupListHandler())

	s.AddTool(mcp.NewTool("nself_deploy_status",
		mcp.WithDescription("Fast local deploy-state read (postgres container presence + control-plane inventory). Not a full SSH-probed remote status — use the CLI for that."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOutputSchema[DeployStatusResult](),
	), mcpDeployStatusHandler())

	s.AddTool(mcp.NewTool("nself_plugin_list",
		mcp.WithDescription("List the plugin catalog: registry entries and/or installed plugins"),
		mcp.WithBoolean("installed", mcp.Description("true = installed only; false/omitted = full registry catalog")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithOutputSchema[PluginListResult](),
	), mcpPluginListHandler())

	s.AddTool(mcp.NewTool("nself_plugin_install",
		mcp.WithDescription("Install one plugin by name (core path only — no --force/--preview/--dry-run)"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Plugin name, e.g. mlflow")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOutputSchema[PluginInstallResult](),
	), mcpPluginInstallHandler())
}

// registerDataMCPTools registers the three Hasura/Postgres data tools —
// see mcp_tools_data.go for the handlers.
func registerDataMCPTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("nself_get_schema",
		mcp.WithDescription("Introspect the Hasura GraphQL schema and return table/type information"),
		mcp.WithReadOnlyHintAnnotation(true),
	), mcpGetSchemaHandler())

	s.AddTool(mcp.NewTool("nself_get_permissions",
		mcp.WithDescription("Return a snapshot of Hasura role permissions for all tables"),
		mcp.WithReadOnlyHintAnnotation(true),
	), mcpGetPermissionsHandler())

	s.AddTool(mcp.NewTool("nself_run_migration",
		mcp.WithDescription("Apply a SQL migration against the nSelf Postgres database. Requires explicit confirmation via the 'confirm' flag. A DDL allowlist blocks destructive statements even with confirm=true."),
		mcp.WithString("sql", mcp.Required(), mcp.Description("SQL to execute as a migration")),
		mcp.WithBoolean("confirm", mcp.Required(), mcp.Description("Must be true to execute — prevents accidental runs")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
	), mcpRunMigrationHandler())
}
