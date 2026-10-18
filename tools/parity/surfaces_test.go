package main

import (
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
)

// TestCountSurfaces pins what the MCP and HTTP columns count: a command and
// every command below it fold into the top-level name, a command the exposure
// rule refuses counts for nothing, and a tool name the generator did not list
// is not credited even when the command is exposed.
func TestCountSurfaces(t *testing.T) {
	row := func(path, side string) cmdregistry.Command {
		return cmdregistry.Command{
			Path: path, Runnable: true, Canon: canon.CanonCore, JSON: canon.JSONEnvelope,
			SideEffect: side, Output: canon.OutputDocument,
		}
	}
	hidden := row("nself db secret", canon.SideEffectRead)
	hidden.Hidden = true
	cliOnly := row("nself config export", canon.SideEffectRead)
	cliOnly.Surface = "cli-only"
	reg := &cmdregistry.Registry{Commands: []cmdregistry.Command{
		row("nself status", canon.SideEffectRead),
		row("nself db", canon.SideEffectRead),
		row("nself db seed", canon.SideEffectRead),
		hidden,
		row("nself config", canon.SideEffectRead),
		cliOnly,
	}}
	tools := map[string]bool{"nself_status": true, "nself_db": true, "nself_db_seed": true, "nself_config_export": true}
	got := countSurfaces(reg, tools)

	want := map[string]surfaceCounts{
		"status": {MCPTools: 1, HTTPRoutes: 1},
		"db":     {MCPTools: 2, HTTPRoutes: 2}, // db, db seed; the hidden row counts for nothing
		"config": {MCPTools: 0, HTTPRoutes: 1}, // config is exposed over HTTP but the generator listed no tool; export is cli-only
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s: got %+v, want %+v", name, got[name], w)
		}
	}
}
