package commands

// mcp_tools_ops.go — read-only MCP tool handlers: config get/show, db
// migrate status, backup list, plugin list, deploy status.
//
// Purpose: CLI-R15. Split out of mcp_tools_core.go once that file crossed
//   the 300-line cap — see its header for the shared design rationale
//   (no stdout writes, call the internal data layer directly).
// Inputs:  MCP tool call arguments.
// Outputs: mcp.CallToolResult with StructuredContent set to a typed result.
// Constraints: config_get/config_show never accept a "reveal" argument —
//   an MCP client is an AI agent, not the operator at the terminal, so
//   secrets are always masked via config.go's maskValue/config.IsSecretKey
//   regardless of what the caller asks for. deploy_status intentionally
//   covers only the fast, local, side-effect-free signal (postgres
//   container presence + control-plane inventory contents) and does not
//   SSH-probe remote servers the way `nself deploy status` does — that
//   stays a CLI-only path so a tool call can't block on network/host
//   timeouts.
// SPORT: CLI-CMD-MCP-001

import (
	"context"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/plugin"
)

// ── config get / show ────────────────────────────────────────────────────

// ConfigGetResult is the structured output of the nself_config_get tool.
type ConfigGetResult struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Redacted bool   `json:"redacted"`
}

func mcpConfigGetHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		key, _ := args["key"].(string)
		if key == "" {
			return mcpErrorResult("Error: key is required")
		}
		envFlag, _ := args["env"].(string)
		if err := validateMCPEnvName(envFlag); err != nil {
			return mcpErrorResult("Error: %v", err)
		}

		projectDir, err := resolveProjectDir()
		if err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		envFile := envFileName(projectDir, envFlag)
		pairs, err := config.ReadEnvFile(envFile)
		if err != nil {
			return mcpErrorResult("Error: %v", config.EnvReadError(envFile, err))
		}
		val, ok := pairs[key]
		if !ok {
			return mcpErrorResult("Error: key not found: %s", key)
		}
		return mcp.NewToolResultStructuredOnly(ConfigGetResult{
			Key:      key,
			Value:    maskValue(key, val, false),
			Redacted: config.IsSecretKey(key) && val != "",
		}), nil
	}
}

// ConfigShowResult is the structured output of the nself_config_show tool.
type ConfigShowResult struct {
	EnvFile string            `json:"env_file"`
	Values  map[string]string `json:"values"`
}

func mcpConfigShowHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		envFlag, _ := req.GetArguments()["env"].(string)
		if err := validateMCPEnvName(envFlag); err != nil {
			return mcpErrorResult("Error: %v", err)
		}

		projectDir, err := resolveProjectDir()
		if err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		envFile := envFileName(projectDir, envFlag)
		pairs, err := config.ReadEnvFile(envFile)
		if err != nil {
			return mcpErrorResult("Error: %v", config.EnvReadError(envFile, err))
		}

		values := make(map[string]string, len(pairs))
		for k, v := range pairs {
			values[k] = maskValue(k, v, false)
		}
		return mcp.NewToolResultStructuredOnly(ConfigShowResult{EnvFile: envFile, Values: values}), nil
	}
}

// ── db migrate status ────────────────────────────────────────────────────

// MigrationStatusEntry mirrors database.MigrationStatus with JSON tags —
// that type has none, since it's only ever printed as a table today.
type MigrationStatusEntry struct {
	Name      string    `json:"name"`
	Applied   bool      `json:"applied"`
	AppliedAt time.Time `json:"applied_at,omitempty"`
}

// MigrateStatusResult wraps the migration status slice.
type MigrateStatusResult struct {
	Migrations []MigrationStatusEntry `json:"migrations"`
}

func mcpDBMigrateStatusHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cfg, err := loadProjectConfig()
		if err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		statuses, err := database.MigrateStatus(ctx, cfg, "")
		if err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		out := make([]MigrationStatusEntry, 0, len(statuses))
		for _, s := range statuses {
			out = append(out, MigrationStatusEntry{Name: s.Name, Applied: s.Applied, AppliedAt: s.Timestamp})
		}
		return mcp.NewToolResultStructuredOnly(MigrateStatusResult{Migrations: out}), nil
	}
}

// ── backup list ──────────────────────────────────────────────────────────

// BackupListResult wraps backup.List's slice.
type BackupListResult struct {
	Backups []backup.BackupEntry `json:"backups"`
}

func mcpBackupListHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cfg, err := loadProjectConfig()
		if err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		entries, err := backup.List(cfg, backup.ListOptions{})
		if err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		return mcp.NewToolResultStructuredOnly(BackupListResult{Backups: entries}), nil
	}
}

// ── plugin list ──────────────────────────────────────────────────────────

// PluginListEntry mirrors plugin.PluginInfo with JSON tags.
type PluginListEntry struct {
	Name          string `json:"name"`
	Version       string `json:"version,omitempty"`
	Category      string `json:"category,omitempty"`
	Installed     bool   `json:"installed"`
	Running       bool   `json:"running"`
	PublishStatus string `json:"publish_status,omitempty"`
}

// PluginListResult wraps the plugin list slice.
type PluginListResult struct {
	Plugins []PluginListEntry `json:"plugins"`
}

func mcpPluginListHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		installed, _ := req.GetArguments()["installed"].(bool)
		plugins, err := plugin.List(resolvePluginDir(), installed)
		if err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		out := make([]PluginListEntry, 0, len(plugins))
		for _, p := range plugins {
			out = append(out, PluginListEntry{
				Name: p.Name, Version: p.Version, Category: p.Category,
				Installed: p.Installed, Running: p.Running, PublishStatus: p.PublishStatus,
			})
		}
		return mcp.NewToolResultStructuredOnly(PluginListResult{Plugins: out}), nil
	}
}

// ── deploy status ────────────────────────────────────────────────────────

// DeployServerInfo is one entry from the control-plane inventory.
type DeployServerInfo struct {
	Env    string `json:"env"`
	Server string `json:"server"`
	Role   string `json:"role"`
	Host   string `json:"host,omitempty"`
}

// DeployStatusResult is the structured output of the nself_deploy_status tool.
type DeployStatusResult struct {
	LocalState string             `json:"local_state"`
	Servers    []DeployServerInfo `json:"servers,omitempty"`
	Note       string             `json:"note"`
}

func mcpDeployStatusHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		state := "unknown"
		if _, err := exec.LookPath("docker"); err == nil {
			out, derr := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}").Output()
			if derr == nil && strings.Contains(string(out), "postgres") {
				state = "running"
			} else {
				state = "not-running"
			}
		}

		result := DeployStatusResult{
			LocalState: state,
			Note:       "local state only; run 'nself deploy status' from the CLI for full SSH-probed remote capability",
		}

		root, err := projectRoot()
		if err == nil {
			if inv, invErr := controlplane.Load(root); invErr == nil {
				for envName, envDef := range inv.Environments {
					for _, srv := range envDef.Servers {
						result.Servers = append(result.Servers, DeployServerInfo{
							Env: envName, Server: srv.Name, Role: string(srv.Role), Host: srv.Host,
						})
					}
				}
			}
		}
		sort.Slice(result.Servers, func(i, j int) bool {
			if result.Servers[i].Env != result.Servers[j].Env {
				return result.Servers[i].Env < result.Servers[j].Env
			}
			return result.Servers[i].Server < result.Servers[j].Server
		})
		return mcp.NewToolResultStructuredOnly(result), nil
	}
}
