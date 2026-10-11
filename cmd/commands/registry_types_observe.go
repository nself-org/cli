// JSON data-type registration for the observe domain (status, doctor, help,
// version).
//
// Purpose: P7-REG-09 pilots (status, doctor: the unchanged pre-contract payload
// plus `state`) and the P7-SURF-11 conversion of the rest of the observe
// fragment (EPIC D10): status urls, status health (and its subcommands except
// watch, which is a stream), doctor heal, doctor images, help topics, version.
// Pre-contract JSON is bare in v1.4 mode and an envelope in v1.5.
// Constraints: registers from init(); see registry_types.go for the rules.
// `status health watch` stays `json: legacy` (output: stream, framed by SURF-27).

package commands

import "github.com/nself-org/cli/internal/health"

func init() {
	registerJSONType("status", statusData{})
	registerJSONType("doctor", doctorData{})
	registerJSONType("status urls", urlsData{})
	registerJSONType("status health", health.HealthReport{})
	registerJSONType("status health check", health.HealthReport{})
	registerJSONType("status health service", health.HealthResult{})
	registerJSONType("status health endpoint", health.HealthResult{})
	registerJSONType("status health history", healthHistoryData{})
	registerJSONType("status health config", healthConfigData{})
	registerJSONType("doctor heal", HealResult{})
	registerJSONType("doctor images", DoctorImages{})
	registerJSONType("help topics", HelpTopicsResult{})
	registerJSONType("version", VersionResult{})
	// The envelope is a v1.5 class for every converted row except doctor
	// images: the pre-contract rows (urls, health, version) keep `json: legacy`
	// in v1.4 (their bytes are pinned by json-conversion-check.sh), and the
	// moved paths (status urls, status health, doctor heal, help topics) exist
	// only in the v1.5 tree. `doctor images --json` is additive in both modes.
	for _, p := range []string{
		"status urls", "status health", "status health check", "status health service",
		"status health endpoint", "status health history", "status health config",
		"doctor heal", "help topics", "version",
	} {
		registerV15OnlyEnvelope(p)
	}
}

// urlsData is the envelope data of `status urls --json`: the unchanged
// pre-contract listing (urlsOutput, embedded). With --diff it is the listing of
// the --env environment and Compared holds the listing of the --diff one (the
// bare v1.4 output of --diff is a map keyed by environment name).
type urlsData struct {
	urlsOutput
	Compared *urlsCompared `json:"compared,omitempty"`
}

// urlsCompared is the second environment of `status urls --diff`.
type urlsCompared struct {
	Env     string     `json:"env"`
	Listing urlsOutput `json:"listing"`
}

// healthHistoryData is the envelope data of `status health history --json`:
// the last 20 reports, oldest first (the bare v1.4 output is the array itself).
type healthHistoryData struct {
	Entries []health.HealthReport `json:"entries"`
}

// healthConfigData is `status health config --json`. Fields are declared in
// key order so the bare v1.4 output stays byte-identical to the map it was.
type healthConfigData struct {
	Env             string `json:"env"`
	IntervalSeconds int    `json:"interval_seconds"`
	JSONOutput      bool   `json:"json_output"`
	Quiet           bool   `json:"quiet"`
	Retries         int    `json:"retries"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
}

// HealAction is one repair routine `doctor heal` ran or would run.
type HealAction struct {
	Name   string `json:"name"`   // routine, e.g. "jwt"
	Status string `json:"status"` // rotated | would-rotate
	Detail string `json:"detail,omitempty"`
}

// HealResult is the envelope data of `doctor heal --json`. It never carries a
// key: a rotated JWT secret is delivered only to --to-file (mode 0600).
type HealResult struct {
	Actions    []HealAction `json:"actions"`
	Rotated    bool         `json:"rotated"`
	DryRun     bool         `json:"dry_run"`
	KeyFile    string       `json:"key_file,omitempty"`
	LogPath    string       `json:"log_path,omitempty"`
	GraceUntil string       `json:"grace_until,omitempty"` // RFC 3339, set when rotated
}

// DoctorImage is one locked image row of `doctor images`.
type DoctorImage struct {
	Name     string `json:"name"`
	Upstream string `json:"upstream"` // present | unreachable | not configured | a diagnosis
	Mirror   string `json:"mirror"`   // same values
	Status   string `json:"status"`   // pass | warn | fail
	Remedy   string `json:"remedy,omitempty"`
}

// DoctorImages is the envelope data of `doctor images --json`.
type DoctorImages struct {
	Images []DoctorImage `json:"images"`
	State  string        `json:"state"` // ok | warnings | unhealthy
}

// HelpTopicEntry is one built-in help topic. Body is set only when a single
// topic was asked for.
type HelpTopicEntry struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Body    string `json:"body,omitempty"`
}

// HelpTopicsResult is the envelope data of `help topics --json`.
type HelpTopicsResult struct {
	Topics []HelpTopicEntry `json:"topics"`
}

// VersionResult is `version --json`. Fields are declared in key order so the
// bare v1.4 output stays byte-identical to the map it was.
type VersionResult struct {
	BuildDate    string   `json:"buildDate"`
	Capabilities []string `json:"capabilities"`
	Commit       string   `json:"commit"`
	GoVersion    string   `json:"goVersion"`
	Platform     string   `json:"platform"`
	Version      string   `json:"version"`
}
