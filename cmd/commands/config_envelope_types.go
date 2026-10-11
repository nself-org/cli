package commands

// Purpose: the v1 envelope data types of the config canon fragment's
// read commands (env, secrets, telemetry, trust, validate; P7-SURF-12).
// Inputs: the facts each command's human output already shows.
// Outputs: values placed in the envelope `data`, registered in
// registry_types_config.go.
// Constraints: no secret value in any type below except the two CLI-only
// rows (secretsGetData, secretsDecryptData), which never cross a machine
// surface. secretsListRow carries names and timestamps only.
// SPORT: P7-SURF-12, contract:cli.json-envelope

// envListRow is one environment of `config env list`.
type envListRow struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// envListData is the data of `config env list`.
type envListData struct {
	Environments []envListRow `json:"environments"`
}

// envPairRow is one key of `config env show`; secret values are redacted by
// internal/env (config.IsSecretKey and config.MaskURLCredentials).
type envPairRow struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// envShowData is the data of `config env show`.
type envShowData struct {
	Environment string       `json:"environment"`
	Pairs       []envPairRow `json:"pairs"`
}

// envDiffRow is one differing key of `config env diff` (redacted like show).
type envDiffRow struct {
	Key    string `json:"key"`
	ValueA string `json:"value_a"` // "" when the key is not set in env A
	ValueB string `json:"value_b"`
}

// envDiffData is the data of `config env diff`.
type envDiffData struct {
	EnvA        string       `json:"env_a"`
	EnvB        string       `json:"env_b"`
	Differences []envDiffRow `json:"differences"`
}

// envExplainFile is one cascade file of `config env explain`.
type envExplainFile struct {
	Name       string `json:"name"`
	Exists     bool   `json:"exists"`
	Precedence int    `json:"precedence"` // 1 is the lowest precedence
}

// envExplainSetter is one file that sets the asked variable.
type envExplainSetter struct {
	File   string  `json:"file"`
	Value  *string `json:"value"` // null unless --reveal
	Winner bool    `json:"winner"`
}

// envExplainData is the data of `config env explain [VAR]`. Key is empty and
// Setters empty for the overview form.
type envExplainData struct {
	Environment       string             `json:"environment"`
	EnvironmentSource string             `json:"environment_source"`
	CascadeMode       string             `json:"cascade_mode"` // canonical | legacy
	Files             []envExplainFile   `json:"files"`
	Key               string             `json:"key"`
	Set               bool               `json:"set"`
	Setters           []envExplainSetter `json:"setters"`
	Revealed          bool               `json:"revealed"`
}

// secretsListRow is one secret of `config secrets list`: name and metadata
// only, never the value.
type secretsListRow struct {
	Name      string `json:"name"`
	UpdatedAt string `json:"updated_at"`
	RotatedAt string `json:"rotated_at"`
}

// secretsListData is the data of `config secrets list`.
type secretsListData struct {
	Environment string           `json:"environment"`
	Secrets     []secretsListRow `json:"secrets"`
}

// secretsAuditRow is one finding of `config secrets audit`.
type secretsAuditRow struct {
	Key      string `json:"key"`
	Issue    string `json:"issue"`
	Severity string `json:"severity"`
}

// secretsAuditData is the data of `config secrets audit`.
type secretsAuditData struct {
	Environment string            `json:"environment"`
	Findings    []secretsAuditRow `json:"findings"`
}

// secretsLintRow is one finding of `config secrets lint`: where and which
// rule. The matched text is never copied; description is the rule's own
// description (empty for a scanner failure whose raw output may hold text).
type secretsLintRow struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Rule        string `json:"rule"`
	Description string `json:"description"`
}

// secretsLintData is the data of `config secrets lint`.
type secretsLintData struct {
	Findings []secretsLintRow `json:"findings"`
}

// secretsScheduleRow is one rotation schedule and its status.
type secretsScheduleRow struct {
	Secret      string `json:"secret"`
	CadenceDays int    `json:"cadence_days"`
	WindowDays  int    `json:"window_days"`
	NextDue     string `json:"next_due"`
	DueInDays   int    `json:"due_in_days"`
	Status      string `json:"status"`
}

// secretsSchedulesData is the data of `config secrets list-schedules`.
type secretsSchedulesData struct {
	Schedules []secretsScheduleRow `json:"schedules"`
}

// secretsScheduleData is the data of `config secrets schedule`: action "set"
// (Secret and CadenceDays say what was stored) or "show" (Schedules).
type secretsScheduleData struct {
	Action      string               `json:"action"`
	Secret      string               `json:"secret"`
	CadenceDays int                  `json:"cadence_days"`
	Schedules   []secretsScheduleRow `json:"schedules"`
}

// secretsRotationLogRow is one event of `config secrets rotation-log`.
type secretsRotationLogRow struct {
	Secret    string `json:"secret"`
	RotatedAt string `json:"rotated_at"`
	Status    string `json:"status"`
	Note      string `json:"note"`
}

// secretsRotationLogData is the data of `config secrets rotation-log`.
type secretsRotationLogData struct {
	Events []secretsRotationLogRow `json:"events"`
}

// secretsGetData is the data of `config secrets get` (CLI-only: it carries
// the decrypted value).
type secretsGetData struct {
	Key         string `json:"key"`
	Environment string `json:"environment"`
	Value       string `json:"value"`
}

// secretsDecryptData is the data of `config secrets decrypt-on-deploy`
// (CLI-only: it carries every decrypted value).
type secretsDecryptData struct {
	Environment string            `json:"environment"`
	Values      map[string]string `json:"values"`
}

// telemetryStatusData is the data of `config telemetry status`.
type telemetryStatusData struct {
	Enabled     bool   `json:"enabled"`
	InstallID   string `json:"install_id"` // anonymous random UUID
	Source      string `json:"source"`
	EnvOverride string `json:"env_override"` // the variable that forces telemetry off, or ""
}

// trustStatusData is the data of `config trust status`. Warnings is true when
// the command exits non-zero (a component is missing or a certificate is
// near expiry).
type trustStatusData struct {
	MkcertInstalled    bool `json:"mkcert_installed"`
	CAInstalled        bool `json:"ca_installed"`
	CertsExist         bool `json:"certs_exist"`
	CertsValid         bool `json:"certs_valid"`
	DNSInstalled       bool `json:"dns_installed"`
	DNSRunning         bool `json:"dns_running"`
	ResolverConfigured bool `json:"resolver_configured"`
	PortsForwarding    bool `json:"ports_forwarding"`
	Warnings           bool `json:"warnings"`
}

// configValidateCheck is one validator of `config validate`.
type configValidateCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Error  string `json:"error"` // "" when passed; credentials in URLs redacted
}

// configValidateData is the data of `config validate`. The document is also
// written when validation fails; the exit status carries the verdict.
type configValidateData struct {
	Valid      bool                  `json:"valid"`
	Checks     []configValidateCheck `json:"checks"`
	Manifest   []string              `json:"manifest_findings"` // nself.yaml findings, one line each
	ErrorCount int                   `json:"error_count"`
}
