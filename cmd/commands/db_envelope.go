package commands

// Purpose: the v1 envelope for the data canon fragment (db, backup, init
// template, update project; P7-SURF-14).
// Inputs: a command body that already ran. Outputs: one envelope on the real
// stdout, only in JSON mode when the registry reports `envelope`.
// Constraints: a body prints its human output as before (the decorator moves
// it to stderr in JSON mode); child-process output is never captured. A body
// with typed facts hands them over with setData; otherwise the shared
// dataResult (or the hub listing) is emitted. The key is the canonical path
// from invokedKey, so the moved commands (migrate -> update project) answer
// under their v1.5 name.
// SPORT: P7-SURF-14, contract:cli.json-envelope

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/seed"
	"github.com/nself-org/cli/internal/templates/clone"
	"github.com/spf13/cobra"
)

// dataResult is the envelope data of a write, remote or destructive command,
// and of a read command whose facts only a child process holds.
type dataResult struct {
	OK     bool   `json:"ok"`
	DryRun bool   `json:"dry_run"`
	Target string `json:"target"` // first positional argument, "" when none
}

// dataHubEntry is one subcommand of a hub.
type dataHubEntry struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// dataHub is the envelope data of a command group: its subcommands.
type dataHub struct {
	Subcommands []dataHubEntry `json:"subcommands"`
}

// dataSlots holds the typed facts a body set for its invocation.
var dataSlots sync.Map

// setData records the typed envelope data of the running command. The type
// must equal the type registered for the command (checked at emit time).
func setData(cmd *cobra.Command, v any) { dataSlots.Store(cmd, v) }

// dataEnv wraps a RunE of the data fragment: after the body succeeds it emits
// the envelope when JSON mode is on and the registry says envelope. A failed
// body emits nothing; the top-level handler prints the error envelope.
func dataEnv(run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		dataSlots.Delete(cmd)
		if err := run(cmd, args); err != nil {
			return err
		}
		return emitDataEnvelope(cmd, args)
	}
}

// jsonEnvelopeOn reports whether this invocation answers with an envelope:
// v1.5 mode, a registered path, and --json (or --format json). Every row of
// this fragment is a v1.5-only envelope with no flag-level JSON override, so
// the registry is not consulted (a plain run never builds it).
func jsonEnvelopeOn(cmd *cobra.Command) bool {
	if !compat.V15() {
		return false
	}
	if _, ok := jsonDataTypes[invokedKey(cmd)]; !ok {
		return false
	}
	if on, err := cmd.Flags().GetBool("json"); err == nil && on {
		return true
	}
	f := cmd.Flags().Lookup("format")
	return f != nil && f.Value.String() == "json"
}

func emitDataEnvelope(cmd *cobra.Command, args []string) error {
	typed, v, set := registeredData(cmd)
	if typed == nil || !jsonEnvelopeOn(cmd) {
		dataSlots.Delete(cmd)
		return nil
	}
	if !set {
		v = defaultData(cmd, args, typed)
	}
	if reflect.TypeOf(v) != reflect.TypeOf(typed) {
		return fmt.Errorf("internal: %s data is %T, the registry type is %T", invokedKey(cmd), v, typed)
	}
	return output.EmitData(output.Default(), invokedKey(cmd), v)
}

func registeredData(cmd *cobra.Command) (typed, v any, set bool) {
	typed = jsonDataTypes[invokedKey(cmd)]
	v, set = dataSlots.LoadAndDelete(cmd)
	return typed, v, set
}

// defaultData builds the shared data for a command that set none.
func defaultData(cmd *cobra.Command, args []string, typed any) any {
	switch typed.(type) {
	case dataHub:
		hub := dataHub{Subcommands: []dataHubEntry{}}
		for _, c := range cmd.Commands() {
			if c.Hidden || !c.IsAvailableCommand() {
				continue
			}
			hub.Subcommands = append(hub.Subcommands, dataHubEntry{Name: c.Name(), Summary: c.Short})
		}
		return hub
	case dataResult:
		res := dataResult{OK: true}
		if f := cmd.Flags().Lookup("dry-run"); f != nil && f.Value.String() == "true" {
			res.DryRun = true
		}
		if len(args) > 0 && !dataTargetSecret[invokedKey(cmd)] {
			res.Target = args[0]
		}
		return res
	}
	return reflect.Zero(reflect.TypeOf(typed)).Interface()
}

// dataTargetSecret lists the paths whose first argument may carry a secret
// (a connection string), so it never reaches the envelope.
var dataTargetSecret = map[string]bool{
	"db import firebase": true, "db import supabase": true,
}

// documentedFailure is an error whose document is already on stdout and whose
// text is already on stderr: the top level prints nothing more (Silent) and
// the exit status stays that of the wrapped error.
type documentedFailure struct{ err error }

func (e documentedFailure) Error() string { return e.err.Error() }
func (e documentedFailure) Unwrap() error { return e.err }
func (e documentedFailure) Silent() bool  { return true }

// codedExit ends a JSON invocation whose document is already written: the
// error keeps its exit status, the human text goes to stderr, and the top
// level prints no second document.
func codedExit(cmd *cobra.Command, err error) error {
	output.RenderError(cmd.ErrOrStderr(), err)
	return documentedFailure{err}
}

// emitEnv writes the envelope of the running command: data in its legacy
// shape goes through EmitLegacyCompatible (NSELF_JSON_LEGACY keeps the bare
// form), a new shape through EmitData. Callers check jsonEnvelopeOn first.
func emitEnv(cmd *cobra.Command, data any, legacyShape bool) error {
	if legacyShape {
		return output.EmitLegacyCompatible(output.Default(), invokedKey(cmd), data)
	}
	return output.EmitData(output.Default(), invokedKey(cmd), data)
}

// newBackupStatusData builds the `backup status` document: the local fields
// (nil outside a project) and the off-box state.
func newBackupStatusData(info *backup.StatusInfo, off *backup.OffboxStatus) backupStatusData {
	d := backupStatusData{}
	if off != nil {
		o := &backupOffbox{Source: off.Source, BackupAgeSeconds: off.BackupAgeSeconds, DrillAgeSeconds: off.DrillAgeSeconds,
			MaxAgeExceeded: off.MaxAgeExceeded, MaxDrillAgeExceeded: off.MaxDrillAgeExceeded, Problems: nonNilStrings(off.Problems)}
		if off.LastBackup != nil {
			o.LastBackup = newDrillHeartbeat(*off.LastBackup)
		}
		if off.LastDrill != nil {
			o.LastDrill = newDrillHeartbeat(*off.LastDrill)
		}
		d.Offbox = o
	}
	if info != nil {
		d.LastRun, d.NextRun, d.Health, d.TotalSize = &info.LastRun, &info.NextRun, &info.Health, &info.TotalSize
		d.BackupCount, d.RetentionDaily = &info.BackupCount, &info.RetentionDaily
		d.RetentionWeekly, d.RetentionMonthly = &info.RetentionWeekly, &info.RetentionMonthly
	}
	return d
}

// newTemplateListData builds the `init template list` document: the embedded
// templates and the community registry entries (empty when it is unreachable).
func newTemplateListData(community []templateEntry) templateListData {
	d := templateListData{Bundled: []templateBundledRow{}, Community: community}
	if d.Community == nil {
		d.Community = []templateEntry{}
	}
	for _, n := range clone.All() {
		if m, err := clone.GetManifest(n); err == nil {
			plugins := m.RequiredPlugins
			if plugins == nil {
				plugins = []string{}
			}
			d.Bundled = append(d.Bundled, templateBundledRow{Name: m.Name, Version: m.Version, Plugins: plugins})
		}
	}
	return d
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// seedRows converts seed files to envelope rows.
func seedRows(seeds []seed.SeedFile) []dbSeedRow {
	rows := []dbSeedRow{}
	for _, s := range seeds {
		kind := "standard"
		if s.Idempotent {
			kind = "idempotent"
		}
		if s.Destructive {
			kind = "destructive"
		}
		rows = append(rows, dbSeedRow{Name: s.Name, Env: s.Env, Type: kind, DependsOn: nonNilStrings(s.DependsOn)})
	}
	return rows
}

// newMigrateStatusData converts the ledger rows of `db migrate status`.
func newMigrateStatusData(statuses []database.MigrationStatus) dbMigrateStatusData {
	d := dbMigrateStatusData{Migrations: []dbMigrationRow{}}
	for _, s := range statuses {
		row := dbMigrationRow{Name: s.Name, Status: "pending"}
		if s.Applied {
			row.Status = "applied"
			if !s.Timestamp.IsZero() {
				t := s.Timestamp.Format(time.RFC3339)
				row.AppliedAt = &t
			}
		}
		d.Migrations = append(d.Migrations, row)
	}
	return d
}

// newDriftScanData converts the Theme 25 scan of `db drift scan`.
func newDriftScanData(results []database.SchemaDriftResult, sum database.DriftSummary) dbDriftScanData {
	d := dbDriftScanData{Tables: []dbDriftTableRow{}, TotalTables: sum.TotalTables, CompliantTables: sum.CompliantTables, DriftedTables: sum.DriftedTables, OverallScore: sum.OverallScore}
	for _, r := range results {
		missing := []string{}
		for _, c := range r.MissingColumns {
			missing = append(missing, c.Name)
		}
		d.Tables = append(d.Tables, dbDriftTableRow{Schema: r.TableSchema, Table: r.TableName, Present: nonNilStrings(r.PresentColumns), Missing: missing, Score: r.DriftScore})
	}
	return d
}

// newDrillHeartbeat copies a heartbeat object into the envelope type.
func newDrillHeartbeat(h backup.Heartbeat) *drillHeartbeat {
	d := &drillHeartbeat{At: h.At, BackupKey: h.BackupKey, Bytes: h.Bytes, CLIVersion: h.CLIVersion, Encrypted: h.Encrypted,
		Kind: h.Kind, Mismatches: nonNilStrings(h.Mismatches), Project: h.Project, Result: h.Result, SchemaVersion: h.SchemaVersion}
	if h.ApproxRows != nil {
		d.ApproxRows = &h.ApproxRows
	}
	if h.RestoredRows != nil {
		d.RestoredRows = &h.RestoredRows
	}
	return d
}
