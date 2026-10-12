// Package progress writes contract:cli.deploy-progress v1: the NDJSON stream
// that `nself deploy --json --stream` and `nself deploy promote <src> <tgt>
// --json --stream` print.
//
// Purpose: let an agent follow a long deploy without parsing human text.
// Inputs: step events from the deploy code, a promote.Observer feed and the
// final outcome.
// Outputs: one compact v1 envelope per line on an io.Writer. Progress lines
// carry an Event as data, the last line carries a Result as data.
// Constraints: every line is built with internal/output and then compacted,
// never hand-built; the Writer is nil-safe so a run without --stream pays
// nothing; no secret or hostname is added by this package.
package progress

// Data types of the two kinds of line (data.type).
const (
	TypeProgress = "progress"
	TypeResult   = "result"
)

// Phases of a progress event (data.phase).
const (
	PhaseStart    = "start"
	PhaseDone     = "done"
	PhaseFailed   = "failed"
	PhaseSkipped  = "skipped"
	PhaseRollback = "rollback"
)

// Outcomes of a run (data.result of the last line).
const (
	ResultOK             = "ok"
	ResultFailed         = "failed"
	ResultRolledBack     = "rolled_back"
	ResultRollbackFailed = "rollback_failed"
)

// Phases lists every accepted data.phase, in contract order.
var Phases = []string{PhaseStart, PhaseDone, PhaseFailed, PhaseSkipped, PhaseRollback}

// Results lists every accepted data.result, in contract order.
var Results = []string{ResultOK, ResultFailed, ResultRolledBack, ResultRollbackFailed}

// Event is the data of one progress line. Field order is the key order on the
// wire (type first, as in the stream framing of contract:cli.json-stream v1).
// Every key is always present; Env, Server and Detail are null when unknown.
type Event struct {
	// Type is always TypeProgress.
	Type string `json:"type"`
	// Seq counts events from 1 and strictly increases within one run.
	Seq int `json:"seq"`
	// Step names the unit of work, for example "Build images" or "apply".
	Step string `json:"step"`
	// Phase is one of Phases.
	Phase string `json:"phase"`
	// Env is the environment the step acts on, or null.
	Env *string `json:"env"`
	// Server is the server the step acts on, or null when not per server.
	Server *string `json:"server"`
	// ElapsedMS is the time since the stream started, in milliseconds.
	ElapsedMS int64 `json:"elapsed_ms"`
	// Detail is a short redacted note (a reason or an error summary), or null.
	Detail *string `json:"detail"`
}

// Step is one row of the final step list.
type Step struct {
	Step   string `json:"step"`
	Status string `json:"status"`
}

// Result is the data of the last line. Steps is never null.
type Result struct {
	// Type is always TypeResult.
	Type string `json:"type"`
	// Result is one of Results.
	Result string `json:"result"`
	// Steps lists each step with its last known status, in first-seen order.
	Steps []Step `json:"steps"`
}

// PhaseForStatus maps a deploy step status to the event phase that reports
// it: failed and unhealthy report failed, pending (a dry-run plan) and skipped
// report skipped, everything else reports done.
func PhaseForStatus(status string) string {
	switch status {
	case "failed", "unhealthy":
		return PhaseFailed
	case "pending", "skipped", "planned":
		return PhaseSkipped
	}
	return PhaseDone
}
