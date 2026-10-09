package commands

// mcp_tools_data.go — Hasura/Postgres data-layer MCP tool handlers.
//
// Purpose: the three tools that predate CLI-R15 and talk to the running
//   Hasura instance directly over HTTP rather than through a `nself`
//   subcommand: schema introspection, permissions snapshot, and gated raw
//   migration SQL. Kept distinct from mcp_tools_core.go because they reach
//   into the project's live database, not the CLI's own command layer.
// Inputs:  MCP tool call arguments; NSELF_HASURA_GRAPHQL_URL/
//   HASURA_GRAPHQL_URL for the endpoint, HASURA_GRAPHQL_ADMIN_SECRET/
//   NSELF_HASURA_ADMIN_SECRET for auth.
// Outputs: mcp.CallToolResult with structured JSON content.
// Constraints: nself_run_migration enforces internal/sqlallowlist before any
//   execution path — see sqlallowlist.ValidateMigrationSQL — blocking DROP/
//   TRUNCATE/DELETE/ALTER ROLE/GRANT/REVOKE/psql meta-commands even when the
//   caller sets confirm=true programmatically.
// SPORT: CLI-CMD-MCP-001

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/hasura"
	"github.com/nself-org/cli/internal/sqlallowlist"
)

// resolveHasuraEndpoint returns the Hasura base URL from env, defaulting to localhost:8080.
func resolveHasuraEndpoint() string {
	return hasura.Endpoint()
}

func mcpGetSchemaHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		hasuraURL := resolveHasuraEndpoint()
		adminSecret := os.Getenv("HASURA_GRAPHQL_ADMIN_SECRET")
		if adminSecret == "" {
			adminSecret = os.Getenv("NSELF_HASURA_ADMIN_SECRET")
		}

		snapshot, err := hasura.Schema(ctx, hasuraURL, adminSecret)
		if err != nil {
			return mcpErrorResult("Schema introspection error: %v", err)
		}

		return mcp.NewToolResultText(hasura.CompactSchema(snapshot.Raw)), nil
	}
}

func mcpGetPermissionsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		hasuraURL := resolveHasuraEndpoint()
		adminSecret := os.Getenv("HASURA_GRAPHQL_ADMIN_SECRET")
		if adminSecret == "" {
			adminSecret = os.Getenv("NSELF_HASURA_ADMIN_SECRET")
		}

		snapshot, err := hasura.Permissions(ctx, hasuraURL, adminSecret)
		if err != nil {
			return mcpErrorResult("Permissions snapshot error: %v", err)
		}

		if !snapshot.Valid {
			return mcp.NewToolResultText(string(snapshot.Raw)), nil
		}
		return mcp.NewToolResultStructuredOnly(snapshot.Tables), nil
	}
}

func mcpRunMigrationHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		sql, _ := args["sql"].(string)
		confirm, _ := args["confirm"].(bool)

		if sql == "" {
			return mcpErrorResult("Error: sql is required")
		}
		if !confirm {
			return mcpErrorResult("Error: confirm must be true to execute the migration. This is a safety gate.")
		}

		// DDL allowlist: reject destructive or privilege-altering SQL before
		// any execution path. This blocks AI Studio sessions from running
		// DROP TABLE, TRUNCATE, DELETE FROM, ALTER ROLE, GRANT/REVOKE, or
		// psql meta-commands even when confirm=true is set programmatically.
		if err := sqlallowlist.ValidateMigrationSQL(sql); err != nil {
			return mcpErrorResult("Error: %v", err)
		}

		raw, err := os.Getwd()
		if err != nil {
			return mcpErrorResult("Error: %v", err)
		}
		cwd, err := config.FindNSelfRoot(raw)
		if err != nil {
			return mcpErrorResult("no nself project found in %s — run 'nself init' first", raw)
		}
		out, err := mcpExecSelf(ctx, cwd, "db", "migrate", "--sql", sql)
		if err != nil {
			out, err = mcpApplyMigrationDirect(ctx, sql)
			if err != nil {
				return mcpErrorResult("Migration failed: %v", err)
			}
		}
		return mcp.NewToolResultText("Migration applied successfully.\n" + out), nil
	}
}

// mcpApplyMigrationDirect applies SQL directly via the Postgres connection string.
// It enforces the DDL allowlist as a defence-in-depth layer even on this fallback
// path — the primary check in mcpRunMigrationHandler runs first, but a second
// guard here ensures no direct caller can bypass it.
func mcpApplyMigrationDirect(ctx context.Context, sql string) (string, error) {
	if err := sqlallowlist.ValidateMigrationSQL(sql); err != nil {
		return "", err
	}

	pgURL := os.Getenv("POSTGRES_URL")
	if pgURL == "" {
		pgURL = os.Getenv("DATABASE_URL")
	}
	if pgURL == "" {
		return "", fmt.Errorf("no POSTGRES_URL or DATABASE_URL set; cannot apply migration directly")
	}
	cmd := exec.CommandContext(ctx, "psql", pgURL, "-c", sql)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
