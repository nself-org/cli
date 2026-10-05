// One schema per envelope command: schemas/commands/<path, spaces as ->.v1.schema.json.
//
// Purpose: every key of cmd/commands.jsonDataTypes (read through
// commands.JSONDataTypes) gets a generated data schema, so a command cannot
// become `json: envelope` without one. cmd/commands/json_contract_test.go
// asserts the files and golden fixtures exist.
package main

import (
	"strings"

	"github.com/nself-org/cli/cmd/commands"
)

// commandOut is the schema path of an envelope command's data type.
func commandOut(path string) string {
	return "commands/" + strings.ReplaceAll(path, " ", "-") + ".v1.schema.json"
}

func init() {
	for path, zero := range commands.JSONDataTypes() {
		Register(Spec{Out: commandOut(path), Type: zero})
	}
}
