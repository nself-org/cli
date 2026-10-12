// JSON data-type registration for the io domain (logs; exec stays interactive).
//
// Purpose: P7-SURF-27. `nself logs --json` answers with one v1 envelope whose
// data is LogsDocument. `nself logs --json --follow` is a stream
// (contract:cli.json-stream v1): one compact envelope per LogRecord, then a
// last line whose data.type is "result" (LogsEnd) or an error envelope. Both
// use the same record shape, so one schema describes a record in either mode
// (schemas/commands/logs.record.v1.schema.json).
// Pre-contract `logs --json` (one JSON object per line) is kept byte for byte
// in v1.4 mode, so the envelope is a v1.5 class (registerV15OnlyEnvelope).
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

// LogRecord is one log line: the data of a stream record line and one member of
// LogsDocument.Records. Type is first so the member order matches the stream
// contract. Service, Time and Level are omitted when the line does not carry them.
type LogRecord struct {
	// Type is always "log".
	Type string `json:"type"`
	// Service is the compose service/container prefix of the line.
	Service string `json:"service,omitempty"`
	// Time is the timestamp the line starts with, as written.
	Time string `json:"time,omitempty"`
	// Level is the detected level: debug, info, warn or error.
	Level string `json:"level,omitempty"`
	// Message is the line without its service prefix and timestamp.
	Message string `json:"message"`
}

// RecordType names the stream record kind (data.type) of a LogRecord.
func (LogRecord) RecordType() string { return logRecordType }

// logRecordType is the data.type of a log record line.
const logRecordType = "log"

// LogsDocument is the data of `nself logs --json` (without --follow). It is not
// LogsResult, which is the structured output of the nself_logs MCP tool.
type LogsDocument struct {
	// Records are the log lines in the order docker printed them.
	Records []LogRecord `json:"records"`
}

// LogsEnd holds the members of the last line of `nself logs --json --follow`,
// beside data.type "result".
type LogsEnd struct {
	// Lines is the number of record lines that were written.
	Lines int `json:"lines"`
}

// jsonRecordTypes maps a command path to the Go type of one stream record's
// data (contract:cli.json-stream v1). tools/schemagen reads it through
// JSONRecordTypes to emit schemas/commands/<path>.record.v1.schema.json. A
// record type implements RecordType() so the schema pins data.type.
var jsonRecordTypes = map[string]any{}

// registerJSONRecordType adds the record type of one streaming command.
func registerJSONRecordType(path string, zero any) {
	if _, dup := jsonRecordTypes[path]; dup {
		panic("commands: JSON record type registered twice for " + path)
	}
	jsonRecordTypes[path] = zero
}

// JSONRecordTypes returns a copy of the registered stream record types, keyed
// by command path without the leading "nself ".
func JSONRecordTypes() map[string]any {
	out := make(map[string]any, len(jsonRecordTypes))
	for k, v := range jsonRecordTypes {
		out[k] = v
	}
	return out
}

func init() {
	registerJSONType("logs", LogsDocument{})
	registerJSONRecordType("logs", LogRecord{})
	registerV15OnlyEnvelope("logs")
}
