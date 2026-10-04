package output

import "github.com/nself-org/cli/internal/errs"

// SchemaVersion is the envelope version, the string "1". It versions the
// envelope only; the shape of Data is each command's own schema.
const SchemaVersion = "1"

// Envelope is the v1 success document: schema_version, command, data, then the
// optional meta. Data is written as null when the command's data is nil.
// Exported so tools/schemagen can generate the schema from it.
type Envelope struct {
	SchemaVersion string `json:"schema_version"`
	Command       string `json:"command"`
	Data          any    `json:"data"`
	Meta          *Meta  `json:"meta,omitempty"`
}

// ErrorEnvelope is the v1 error document: schema_version, command, error, then
// the optional meta. It has no data key, so exactly one of data/error exists
// in any document. Exported so tools/schemagen can generate the schema.
type ErrorEnvelope struct {
	SchemaVersion string       `json:"schema_version"`
	Command       string       `json:"command"`
	Error         *errs.Detail `json:"error"`
	Meta          *Meta        `json:"meta,omitempty"`
}

// Meta is the optional last member of an envelope. Both lists are omitted when
// empty and the whole member is omitted when both are empty.
type Meta struct {
	Deprecations []Deprecation `json:"deprecations,omitempty"`
	Warnings     []string      `json:"warnings,omitempty"`
}

// Deprecation is one deprecation notice: the old invocation, its replacement
// and the release at which the old form is removed.
type Deprecation struct {
	Old       string `json:"old"`
	New       string `json:"new"`
	RemovalAt string `json:"removal_at"`
}
