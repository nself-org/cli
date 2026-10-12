package commands

// Purpose: the data types of the v1 envelopes of the deploy canon fragment
// (deploy, deploy status|health|check-access|environments|access|ops|web|
// rollback|security, doctor security, update images|upgrade|verify-sbom;
// P7-SURF-15). Registered in registry_types_deploy.go.
// Inputs: facts the command bodies already hold. Outputs: JSON-tagged values.
// Constraints: no secret, key material or child-process output is ever put in
// a field; every list is non-nil so the document never carries null.
// SPORT: P7-SURF-15, contract:cli.json-envelope

// deployData is the envelope data of `deploy`: the plan of steps under
// --dry-run, the steps and per-server outcomes of a real run. A failed run
// prints the error envelope instead (the run error carries the reason).
type deployData struct {
	Target     string            `json:"target"`
	Strategy   string            `json:"strategy"`
	DryRun     bool              `json:"dry_run"`
	Steps      []deployStep      `json:"steps"`
	Servers    []deployServerRow `json:"servers"`
	DurationMS int64             `json:"duration_ms"`
	Success    bool              `json:"success"`
}

// deployServerRow is one server of a topology plan or pipeline run. Status is
// planned (dry run), ok or skipped.
type deployServerRow struct {
	Env        string `json:"env"`
	Server     string `json:"server"`
	Role       string `json:"role,omitempty"`
	Status     string `json:"status"`
	Capability string `json:"capability,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// deployStatusData is the envelope data of `deploy status`.
type deployStatusData struct {
	Target  string               `json:"target"`
	State   string               `json:"state"`
	Servers []deployStatusServer `json:"servers"`
}

// deployStatusServer is one probed server of `deploy status`.
type deployStatusServer struct {
	Env        string `json:"env"`
	Server     string `json:"server"`
	Role       string `json:"role"`
	Capability string `json:"capability"`
	Reason     string `json:"reason,omitempty"`
	LatencyMS  int    `json:"latency_ms,omitempty"`
}

// deployCheckAccessData is the envelope data of `deploy check-access`.
type deployCheckAccessData struct {
	Servers []deployAccessRow `json:"servers"`
	AllOK   bool              `json:"all_ok"`
}

// deployAccessRow is one server of `deploy check-access`.
type deployAccessRow struct {
	Env        string `json:"env"`
	Server     string `json:"server"`
	Capability string `json:"capability"`
	Reason     string `json:"reason,omitempty"`
}

// accessListData is the envelope data of `deploy access list`: one row per
// resolved host (a single --host target is one row).
type accessListData struct {
	Hosts []accessHostRow `json:"hosts"`
}

// securityFinding is one check of the host security audit.
type securityFinding struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// securityAuditData is the envelope data of `doctor security audit`.
type securityAuditData struct {
	Findings []securityFinding `json:"findings"`
	Passed   int               `json:"passed"`
	Failed   int               `json:"failed"`
}

// securityStatusData is the envelope data of `doctor security status`.
type securityStatusData struct {
	Findings []securityFinding `json:"findings"`
	Score    int               `json:"score"`
	Total    int               `json:"total"`
	Percent  int               `json:"percent"`
}

// securitySetupStep is one planned hardening action.
type securitySetupStep struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
}

// securitySetupData is the envelope data of `deploy security setup`. Without
// --apply nothing changed (dry_run true); Supported is false off Linux.
type securitySetupData struct {
	DryRun    bool                `json:"dry_run"`
	Supported bool                `json:"supported"`
	Applied   bool                `json:"applied"`
	Steps     []securitySetupStep `json:"steps"`
}

// updateImageRow is one image digest of `update images`.
type updateImageRow struct {
	Service string `json:"service"`
	Digest  string `json:"digest"`
	Status  string `json:"status"` // new | updated | unchanged
}

// updateImagesData is the envelope data of `update images`.
type updateImagesData struct {
	DryRun bool             `json:"dry_run"`
	Saved  bool             `json:"saved"`
	Images []updateImageRow `json:"images"`
}

// updateUpgradeData is the envelope data of `update upgrade`: the operation
// that was asked for and the version the binary had when it started.
type updateUpgradeData struct {
	OK             bool   `json:"ok"`
	Action         string `json:"action"` // check | rollback | binary-url | upgrade
	CurrentVersion string `json:"current_version"`
	TargetVersion  string `json:"target_version,omitempty"`
}

// verifySBOMData is the envelope data of `update verify-sbom`. A failed
// verification is an error envelope, so Verified is always true here.
type verifySBOMData struct {
	Version  string `json:"version"`
	Repo     string `json:"repo"`
	Verified bool   `json:"verified"`
}
