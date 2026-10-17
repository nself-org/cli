package commands

// Purpose: D-0528 tests for the needs-upgrade flag of `db migrate status`
// through the MCP db_migrate_status tool, plus the docker double both the
// envelope and the MCP test use.
// Inputs: a docker stub on PATH that reports a legacy ledger, a temp project.
// Outputs: pass or fail. Constraints: POSIX shell stub, no network.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// writeUpgradeDocker writes a docker double whose ledger holds two legacy
// nself_ops ids (so the ledger needs its upgrade) and one applied migration,
// and returns the directory to put on PATH.
func writeUpgradeDocker(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the docker double is a POSIX shell script")
	}
	bin := t.TempDir()
	stub := "#!/bin/sh\nfor a; do last=\"$a\"; done\ncase \"$last\" in\n" +
		"  *\"to_regclass(\"*) echo yes ;;\n" +
		"  *\"count(*) FROM nself_ops.migrations\"*) echo 2 ;;\n" +
		"  *\"name || '|' || applied_at\"*) printf '20260101_init.sql|2026-01-02T03:04:05Z\\n' ;;\nesac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestMCPDBMigrateStatus_CarriesLedgerNeedsUpgrade(t *testing.T) {
	prependPath(t, writeUpgradeDocker(t))
	proj := dataProject(t)
	if err := os.MkdirAll(filepath.Join(proj, "migrations"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "migrations", "20260101_init.sql"), []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := mcpDBMigrateStatusHandler()(context.Background(), mcp.CallToolRequest{})
	if err != nil || res.IsError {
		t.Fatalf("handler: err=%v res=%+v", err, res)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["ledger_needs_upgrade"]) != "true" {
		t.Fatalf("ledger_needs_upgrade = %s, want true\n%s", got["ledger_needs_upgrade"], raw)
	}
}
