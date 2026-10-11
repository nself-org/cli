package commands

// Purpose: builders for the envelope data of the `config secrets` commands
// (P7-SURF-12), kept apart from the cobra declarations so the secret-handling
// rules sit in one place.
// Inputs: results of internal/secrets calls the command bodies already made.
// Outputs: the typed documents of config_envelope_types.go.
// Constraints: names and metadata only. newSecretsListData copies name,
// updated_at and rotated_at by field and never reads SecretEntry.Value; the
// lint builder never copies scanner output into a description. The two
// decrypting commands (get, decrypt-on-deploy) are CLI-only rows.
// SPORT: P7-SURF-12, contract:cli.json-envelope

import (
	"strings"

	"github.com/nself-org/cli/internal/secrets"
)

// newSecretsListData builds `config secrets list`: one row per key in keys,
// with the entry's timestamps. The value field of an entry is not read.
func newSecretsListData(env string, keys []string, entries map[string]secrets.SecretEntry) secretsListData {
	d := secretsListData{Environment: env, Secrets: []secretsListRow{}}
	for _, k := range keys {
		e := entries[k]
		d.Secrets = append(d.Secrets, secretsListRow{Name: k, UpdatedAt: e.UpdatedAt, RotatedAt: e.RotatedAt})
	}
	return d
}

// newSecretsAuditData builds `config secrets audit`.
func newSecretsAuditData(env string, findings []secrets.AuditFinding) secretsAuditData {
	d := secretsAuditData{Environment: env, Findings: []secretsAuditRow{}}
	for _, f := range findings {
		d.Findings = append(d.Findings, secretsAuditRow{Key: f.Key, Issue: f.Issue, Severity: f.Severity})
	}
	return d
}

// lintParseError is the rule of the placeholder finding LintSecrets returns
// when the scanner report cannot be parsed; its Message is the raw report.
const lintParseError = "parse-error"

// newSecretsLintData builds `config secrets lint`: file, line, rule and the
// rule's description. The description of a parse-error placeholder holds raw
// scanner output (which may contain the matched text), so it is dropped.
func newSecretsLintData(findings []secrets.LintFinding) secretsLintData {
	d := secretsLintData{Findings: []secretsLintRow{}}
	for _, f := range findings {
		row := secretsLintRow{File: f.File, Line: f.Line, Rule: f.Rule, Description: f.Message}
		if f.Rule == lintParseError {
			row.Description = ""
		}
		d.Findings = append(d.Findings, row)
	}
	return d
}

// newScheduleRows converts the schedule checks of `secrets schedule`.
func newScheduleRows(checks []secrets.ScheduleCheck) []secretsScheduleRow {
	rows := []secretsScheduleRow{}
	for _, c := range checks {
		rows = append(rows, secretsScheduleRow{Secret: c.SecretName, CadenceDays: c.CadenceDays, WindowDays: c.WindowDays,
			NextDue: c.NextDue, DueInDays: c.DueInDays, Status: c.Status})
	}
	return rows
}

// newSecretsRotationLogData builds `config secrets rotation-log`, filtered by
// secret name when filter is set (as the table is).
func newSecretsRotationLogData(events []secrets.RotationLogEntry, filter string) secretsRotationLogData {
	d := secretsRotationLogData{Events: []secretsRotationLogRow{}}
	for _, e := range events {
		if filter != "" && e.SecretName != filter {
			continue
		}
		d.Events = append(d.Events, secretsRotationLogRow{Secret: e.SecretName, RotatedAt: e.RotatedAt, Status: e.Status, Note: e.Note})
	}
	return d
}

// newSecretsDecryptData parses the KEY=VALUE lines DecryptForDeploy prints.
func newSecretsDecryptData(env, text string) secretsDecryptData {
	d := secretsDecryptData{Environment: env, Values: map[string]string{}}
	for _, line := range strings.Split(text, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && k != "" {
			d.Values[k] = v
		}
	}
	return d
}
