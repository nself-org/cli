package admin

// Purpose: the v1 envelope for the admin family (`admin`, `admin start|stop|
// health|logs`, `admin projects ...`; P7-SURF-13). The data types are exported
// so cmd/commands registers them (registry_types_admin.go): the registry, the
// schemas and the goldens live there, the emission lives here.
// Inputs: a command that already ran, plus the facts it knows.
// Outputs: one envelope on the real stdout, in v1.5 JSON mode only (the
// invocation decorator has already moved human output to stderr then).
// Constraints: no new behaviour and no secrets: URLs are the local admin URL,
// project records carry no credentials. `admin connect` and `admin serve` are
// streams and do not use this file.
// SPORT: P7-SURF-13, contract:cli.json-envelope

import (
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// Hub is the data of an admin command group: its subcommands.
type Hub struct {
	// Subcommands are the available subcommands in registration order.
	Subcommands []HubEntry `json:"subcommands"`
}

// HubEntry is one subcommand of a Hub.
type HubEntry struct {
	// Name is the subcommand name.
	Name string `json:"name"`
	// Summary is its one-line description.
	Summary string `json:"summary"`
}

// OpenData is the data of `admin`: where the dashboard lives and whether a
// browser was launched for it.
type OpenData struct {
	// URL is the local Admin URL.
	URL string `json:"url"`
	// Opened is true when a browser was started (false in headless runs).
	Opened bool `json:"opened"`
}

// StartData is the data of `admin start`.
type StartData struct {
	// Listen is the host:port Admin is bound to.
	Listen string `json:"listen"`
	// ExposeNetwork is true when it is bound beyond loopback.
	ExposeNetwork bool `json:"expose_network"`
}

// StopData is the data of `admin stop`.
type StopData struct {
	// Container is the stopped container.
	Container string `json:"container"`
	// Force is true when the graceful drain was skipped.
	Force bool `json:"force"`
}

// HealthData is the data of `admin health` (an unhealthy Admin is an error).
type HealthData struct {
	// URL is the probed health URL.
	URL string `json:"url"`
	// Healthy is always true in a success envelope.
	Healthy bool `json:"healthy"`
	// StatusCode is the HTTP status the probe got.
	StatusCode int `json:"status_code"`
}

// LogsData is the data of `admin logs` without --follow.
type LogsData struct {
	// Container is the Admin container the lines came from.
	Container string `json:"container"`
	// Lines are the log lines in the order docker printed them.
	Lines []string `json:"lines"`
}

// LogRecord is one line of `admin logs --json --follow` (contract:cli.json-stream v1).
type LogRecord struct {
	// Type is always "log".
	Type string `json:"type"`
	// Message is the log line.
	Message string `json:"message"`
}

// RecordType names the stream record kind (data.type) of a LogRecord.
func (LogRecord) RecordType() string { return "log" }

// LogsEnd holds the members of the last line of `admin logs --json --follow`.
type LogsEnd struct {
	// Lines is the number of record lines written.
	Lines int `json:"lines"`
}

// ProjectChange is the data of `admin projects add|remove`.
type ProjectChange struct {
	// ID is the project id that was added or removed.
	ID string `json:"id"`
	// Action is "added" or "removed".
	Action string `json:"action"`
}

// ProjectList is the data of `admin projects list`: the saved projects.
type ProjectList []AdminProject

// jsonOn reports whether this run answers with an envelope: v1.5 and --json.
func jsonOn(cmd *cobra.Command) bool {
	// compat.V15(P7-SURF-13): --json on the admin family -> v1 envelope (admin projects list printed a bare array)
	if !compat.V15() {
		return false
	}
	on, err := cmd.Flags().GetBool("json")
	return err == nil && on
}

// commandKey is the canonical path without the root name ("admin health").
func commandKey(cmd *cobra.Command) string {
	return strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), cmd.Root().CommandPath()))
}

// emit writes the envelope of cmd. legacy marks a shape that printed JSON
// before the contract (NSELF_JSON_LEGACY=1 keeps it bare).
func emit(cmd *cobra.Command, data any, legacy bool) error {
	if legacy {
		return output.EmitLegacyCompatible(output.Default(), commandKey(cmd), data)
	}
	return output.EmitData(output.Default(), commandKey(cmd), data)
}

// hubOf lists the visible subcommands of a group.
func hubOf(cmd *cobra.Command) Hub {
	h := Hub{Subcommands: []HubEntry{}}
	for _, c := range cmd.Commands() {
		if c.Hidden || !c.IsAvailableCommand() {
			continue
		}
		h.Subcommands = append(h.Subcommands, HubEntry{Name: c.Name(), Summary: c.Short})
	}
	return h
}

// withEnvelope wraps a body: after it succeeds, in JSON mode, it emits the
// data facts returns. A body that failed emits nothing (the top-level handler
// prints the error envelope).
func withEnvelope(run func(*cobra.Command, []string) error, facts func(*cobra.Command, []string) any) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := run(cmd, args); err != nil {
			return err
		}
		if !jsonOn(cmd) {
			return nil
		}
		return emit(cmd, facts(cmd, args), false)
	}
}
