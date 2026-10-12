package main

import "github.com/nself-org/cli/internal/deploy/progress"

// deploy-progress.v1.schema.json is the data of one progress line of
// `nself deploy --json --stream` (contract:cli.deploy-progress v1, P7-DEPL-17).
// The last line of the stream carries progress.Result, which is not registered:
// the contract publishes the event schema only.
func init() {
	Register(Spec{Out: "deploy-progress.v1.schema.json", Type: progress.Event{}, Overrides: []Override{
		{Pointer: "/properties/type", Set: map[string]any{"const": progress.TypeProgress}},
		{Pointer: "/properties/seq", Set: map[string]any{"minimum": 1}},
		{Pointer: "/properties/phase", Set: map[string]any{"enum": progress.Phases}},
		{Pointer: "/properties/elapsed_ms", Set: map[string]any{"minimum": 0}},
	}})
}
