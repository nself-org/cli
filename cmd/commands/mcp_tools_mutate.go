package commands

// mcp_tools_mutate.go — state-changing MCP tool handlers.
//
// Purpose: CLI-R15's non-read-only tools: config writes and lifecycle
//   operations (build/start/stop/restart/plugin install). See mcp_exec.go
//   for why build/start/stop/restart/plugin-install shell out to the nself
//   binary itself via os.Executable() instead of calling the cmd/commands
//   RunE functions in-process.
// Inputs:  MCP tool call arguments.
// Outputs: mcp.CallToolResult with a structured ExecResult (command run,
//   captured output, exit success) for the re-exec'd tools, or a typed
//   result for config_set and plugin_install (which call internal packages
//   directly — see below).
// Constraints: config_set reuses config.go's own validateConfigKey/
//   validateConfigValue/setEnvFileLine (pure, non-printing) rather than
//   runConfigSet, which prints a ui.Success line to stdout. plugin_install
//   reuses internal/plugin's exported, non-printing Install/CheckEOLBlock/
//   ValidateNetworkAccess directly rather than runPluginInstall, which is
//   400+ lines covering flags (--force/--preview/--dry-run/--show-graph/
//   free-account registration/telemetry) that don't apply to a single
//   MCP-driven install — this covers the core path only.
// SPORT: CLI-CMD-MCP-001

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
)

// ExecResult is the structured output of every re-exec'd lifecycle tool
// (build/start/stop/restart).
type ExecResult struct {
	Command string `json:"command"`
	Output  string `json:"output"`
	Success bool   `json:"success"`
}

// mcpLifecycleHandler builds a handler that re-execs `nself <args...>` in the
// current project's root and reports the outcome. Shared by build/start/
// stop/restart since they differ only in the subcommand.
func mcpLifecycleHandler(args ...string) server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, cwd, err := mcpLoadProject()
		if err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		out, runErr := mcpExecSelf(ctx, cwd, args...)
		result := ExecResult{
			Command: "nself " + strings.Join(args, " "),
			Output:  out,
			Success: runErr == nil,
		}
		if runErr != nil {
			result.Output = fmt.Sprintf("%s\n(exit error: %v)", out, runErr)
		}
		return mcp.NewToolResultStructuredOnly(result), nil
	}
}

// ── config set ───────────────────────────────────────────────────────────

// ConfigSetResult is the structured output of the nself_config_set tool.
type ConfigSetResult struct {
	Key     string `json:"key"`
	File    string `json:"file"`
	Created bool   `json:"created"`
}

func mcpConfigSetHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		key, _ := args["key"].(string)
		value, _ := args["value"].(string)
		envFlag, _ := args["env"].(string)

		if err := errors.Join(validateConfigKey(key), config.RefuseLegacyMCPSecretSet(key)); err != nil { // D-0737
			return mcpErrorResult("Error: %v", err)
		}
		if err := validateConfigValue(value); err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		if err := validateMCPEnvName(envFlag); err != nil {
			return mcpErrorResult("Error: %v", err)
		}

		projectDir, err := resolveProjectDir()
		if err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		envFile := envFileName(projectDir, envFlag)
		updated, err := setEnvFileLine(envFile, key, value)
		if err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		return mcp.NewToolResultStructuredOnly(ConfigSetResult{Key: key, File: envFile, Created: !updated}), nil
	}
}

// ── plugin install ───────────────────────────────────────────────────────

// PluginInstallResult is the structured output of the nself_plugin_install tool.
type PluginInstallResult struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Message   string `json:"message,omitempty"`
}

func mcpPluginInstallHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.GetArguments()["name"].(string)
		if name == "" {
			return mcpErrorResult("Error: name is required")
		}

		if err := plugin.ValidateNetworkAccess(ctx, ""); err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		if err := plugin.CheckEOLBlock(ctx, name, false); err != nil {
			return mcp.NewToolResultStructuredOnly(PluginInstallResult{Name: name, Installed: false, Message: err.Error()}), nil
		}

		cfg, err := loadConfig()
		if err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		pluginDir := resolvePluginDir()
		if err := installPluginWithOperation(ctx, cfg, name, pluginDir); err != nil {
			return mcp.NewToolResultStructuredOnly(PluginInstallResult{Name: name, Installed: false, Message: err.Error()}), nil
		}
		return mcp.NewToolResultStructuredOnly(PluginInstallResult{Name: name, Installed: true}), nil
	}
}
