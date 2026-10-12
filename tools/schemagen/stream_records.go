package main

// Purpose: one record schema per streaming command (contract:cli.json-stream
// v1, P7-SURF-27): schemas/commands/<path>-record.v1.schema.json describes the
// data object of one record line (data.type names the kind). The contract text
// names <path>.record.v1.schema.json, but spec.go outPattern (outside this
// Ticket's scope) rejects a dot in the name; recordOut is the one place to
// switch once the pattern allows it.
// Inputs: cmd/commands.JSONRecordTypes (path -> record zero value). A record
// type with RecordType() string gets that value as the const of its type member.
// Outputs: one Spec per path.
// Constraints: model of deploy_progress.go; the last line of a stream
// (data.type "result") is not registered, the contract publishes records only.

import (
	"strings"

	"github.com/nself-org/cli/cmd/commands"
)

// recordTyper is implemented by record types that pin their data.type.
type recordTyper interface{ RecordType() string }

// recordOut is the schema path of a command's stream record type.
func recordOut(path string) string {
	return "commands/" + strings.ReplaceAll(path, " ", "-") + "-record.v1.schema.json"
}

func init() {
	for path, zero := range commands.JSONRecordTypes() {
		spec := Spec{Out: recordOut(path), Type: zero}
		if rt, ok := zero.(recordTyper); ok {
			spec.Overrides = []Override{{Pointer: "/properties/type", Set: map[string]any{"const": rt.RecordType()}}}
		}
		Register(spec)
	}
}
