package commands

// Purpose: tests for the data fragment's v1 envelopes (P7-SURF-14): coverage
// of the canon rows, the shared dataResult/dataHub emission, the backup
// status and drill documents, and the golden writer.
// Inputs: package-level commands driven in v1.5 mode with --json set.
// Constraints: no network; backup state lives in path:// directories.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// testPkgDir is this package directory, taken before a test changes directory.
var testPkgDir, _ = os.Getwd()

// TestEnvelopeCoverageData: every runnable document row of the data canon
// fragment is an envelope command.
func TestEnvelopeCoverageData(t *testing.T) {
	assertFragmentEnvelopeCoverage(t, "data")
}

// jsonV15 puts the process in v1.5 mode and the command in --json mode, runs
// fn with stdout captured, and restores every flag it touched.
func jsonV15(t *testing.T, cmd *cobra.Command, flags map[string]string, fn func() error) (string, error) {
	t.Helper()
	compattest.Set(t, true)
	resetRegistryCache()
	t.Cleanup(resetRegistryCache)
	cmd.InheritedFlags()
	fs := cmd.Flags()
	saved := map[string]string{}
	for k, v := range flags {
		f := fs.Lookup(k)
		if f == nil {
			t.Fatalf("flag --%s missing on %s", k, cmd.CommandPath())
		}
		saved[k] = f.Value.String()
		if err := fs.Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for k, v := range saved {
			_ = fs.Set(k, v)
			fs.Lookup(k).Changed = false
		}
	})
	return captureStdout(t, fn)
}

// oneEnvelope decodes stdout as exactly one success envelope and validates it
// against the envelope schema and the command's data schema.
func oneEnvelope(t *testing.T, out, command string) map[string]json.RawMessage {
	t.Helper()
	t.Chdir(testPkgDir) // the schema loader reads ../../schemas
	dec := json.NewDecoder(strings.NewReader(out))
	var doc map[string]json.RawMessage
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("stdout is not a document: %v\n%s", err, out)
	}
	if dec.More() {
		t.Fatalf("more than one document on stdout:\n%s", out)
	}
	if err := contractValidate(contractResolve(t, "envelope.v1.schema.json"), []byte(out)); err != nil {
		t.Fatalf("envelope schema: %v\n%s", err, out)
	}
	if err := contractValidate(contractResolve(t, schemaFileFor(command)), doc["data"]); err != nil {
		t.Fatalf("%s data schema: %v\n%s", command, err, out)
	}
	return doc
}

func TestBackupStatusJSON(t *testing.T) {
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=proj\nENV=dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)
	t.Setenv("NSELF_BACKUP_HEARTBEAT_REMOTE", "")
	out, err := jsonV15(t, backupStatusCmd, map[string]string{"json": "true"}, func() error { return runBackupStatus(backupStatusCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	doc := oneEnvelope(t, out, "backup status")
	var data map[string]json.RawMessage
	if err := json.Unmarshal(doc["data"], &data); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"last_run", "next_run", "health", "total_size", "backup_count", "retention_daily", "offbox"} {
		if data[k] == nil {
			t.Errorf("data.%s missing:\n%s", k, out)
		}
	}
	if !strings.Contains(string(data["offbox"]), `"source": "none"`) || !strings.Contains(string(data["offbox"]), `"problems": []`) {
		t.Errorf("offbox: %s", data["offbox"])
	}
}

func TestBackupStatusProjectOffboxOnly(t *testing.T) {
	hb := t.TempDir()
	t.Chdir(t.TempDir()) // not a project
	writeStatusHB(t, hb, "backup", 27*time.Hour, "ok")
	flags := map[string]string{"json": "true", "project": "proj", "heartbeat-to": "path://" + hb, "max-age": "26h"}
	var runErr error
	out, err := jsonV15(t, backupStatusCmd, flags, func() error {
		runErr = runBackupStatus(backupStatusCmd, nil)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The stale backup fails with E217 and still leaves one document.
	if runErr == nil || !strings.Contains(runErr.Error(), "exit status") {
		t.Fatalf("a stale backup must keep a non-zero exit, got %v", runErr)
	}
	doc := oneEnvelope(t, out, "backup status")
	var data map[string]json.RawMessage
	if err := json.Unmarshal(doc["data"], &data); err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 || data["offbox"] == nil || !strings.Contains(string(data["offbox"]), `"max_age_exceeded": true`) {
		t.Fatalf("outside a project only offbox is present:\n%s", out)
	}
}

func TestBackupDrillJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the docker and age doubles are POSIX shell scripts")
	}
	root := t.TempDir()
	bin, src, hbDir := filepath.Join(root, "bin"), filepath.Join(root, "src"), filepath.Join(root, "hb")
	for _, d := range []string{bin, src, filepath.Join(hbDir, "surf")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{"docker": drillFakeDocker, "age": "#!/bin/sh\ncat\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DOCKER_DIR", root)
	t.Setenv("TMPDIR", t.TempDir())
	identity := filepath.Join(root, "id.key")
	key := "surf_stream_20261005_023000.sql.age"
	for p, body := range map[string]string{identity: "AGE-SECRET-KEY-FAKE\n", filepath.Join(src, key): "PGDMP-fake",
		filepath.Join(hbDir, "surf", "backup.json"): `{"schema_version":"1","kind":"backup","project":"surf","at":"` + time.Now().UTC().Format(time.RFC3339) +
			`","result":"ok","backup_key":"` + key + `","bytes":10,"encrypted":true,"cli_version":"1.4.12","approx_rows":{"public.users":7},"restored_rows":null,"mismatches":[]}`} {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(t.TempDir())
	t.Setenv("NSELF_BACKUP_HEARTBEAT_REMOTE", "")
	flags := map[string]string{"json": "true", "project": "surf", "from": "path://" + src, "identity": identity, "heartbeat-to": "path://" + hbDir}
	out, err := jsonV15(t, backupDrillCmd, flags, func() error { return runBackupDrill(backupDrillCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	doc := oneEnvelope(t, out, "backup drill")
	var got, want struct {
		Local  *json.RawMessage `json:"local"`
		Remote *drillHeartbeat  `json:"remote"`
	}
	golden, err := os.ReadFile(fixtureFileFor("backup drill"))
	if err != nil {
		t.Fatal(err)
	}
	var gdoc map[string]json.RawMessage
	if err := json.Unmarshal(golden, &gdoc); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(doc["data"], &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(gdoc["data"], &want); err != nil {
		t.Fatal(err)
	}
	if got.Local != nil || got.Remote == nil || got.Remote.Result != "ok" || got.Remote.Kind != "drill" || (*got.Remote.RestoredRows)["public.users"] != 7 {
		t.Fatalf("drill document: %s", out)
	}
	if want.Remote == nil || want.Remote.Kind != got.Remote.Kind || want.Remote.Result != got.Remote.Result || want.Remote.SchemaVersion != got.Remote.SchemaVersion {
		t.Fatalf("golden and run disagree: golden %+v run %+v", want.Remote, got.Remote)
	}
}

// TestDataEnvelopeShared: the shared result and hub documents, the type check
// and the v1.4 silence.
func TestDataEnvelopeShared(t *testing.T) {
	out, err := jsonV15(t, dbDropCmd, map[string]string{"json": "true"}, func() error { return emitDataEnvelope(dbDropCmd, []string{"mydb"}) })
	if err != nil {
		t.Fatal(err)
	}
	var res dataResult
	if err := json.Unmarshal(oneEnvelope(t, out, "db drop")["data"], &res); err != nil || !res.OK || res.Target != "mydb" {
		t.Fatalf("result %+v %v\n%s", res, err, out)
	}
	out, err = jsonV15(t, backupCmd, map[string]string{"json": "true"}, func() error { return emitDataEnvelope(backupCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var hub dataHub
	if err := json.Unmarshal(oneEnvelope(t, out, "backup")["data"], &hub); err != nil || len(hub.Subcommands) < 5 {
		t.Fatalf("hub %+v %v\n%s", hub, err, out)
	}
	// A body that hands over the wrong type is a programming error, not output.
	_, err = jsonV15(t, dbDropCmd, map[string]string{"json": "true"}, func() error {
		setData(dbDropCmd, dbVerifyData{})
		return emitDataEnvelope(dbDropCmd, nil)
	})
	if err == nil || !strings.Contains(err.Error(), "internal:") {
		t.Fatalf("wrong data type accepted: %v", err)
	}
	// v1.4 mode and plain human mode print no document.
	compattest.Set(t, false)
	resetRegistryCache()
	stale, err := captureStdout(t, func() error { return emitDataEnvelope(dbDropCmd, nil) })
	if err != nil || stale != "" {
		t.Fatalf("v1.4 emitted %q %v", stale, err)
	}
}

// TestDataGoldenWrite regenerates the goldens of the data fragment; it runs
// only with NSELF_UPDATE_GOLDEN=1.
func TestDataGoldenWrite(t *testing.T) {
	if os.Getenv("NSELF_UPDATE_GOLDEN") != "1" {
		t.Skip("set NSELF_UPDATE_GOLDEN=1 to rewrite the data goldens")
	}
	for path, zero := range dataEnvelopeTypes {
		var buf bytes.Buffer
		if err := output.EmitData(output.Writer{Out: &buf, Err: &bytes.Buffer{}}, path, goldenSample(path, zero)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixtureFileFor(path), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func goldenSample(path string, zero any) any {
	switch path {
	case "backup drill":
		return backupDrillData{Remote: newDrillHeartbeat(backup.Heartbeat{ApproxRows: nil, At: "2026-10-05T03:00:00Z", BackupKey: "surf_stream_20261005_023000.sql.age",
			Bytes: 10, CLIVersion: "1.5.0", Encrypted: true, Kind: "drill", Mismatches: []string{}, Project: "surf",
			RestoredRows: map[string]int64{"public.users": 7}, Result: "ok", SchemaVersion: "1"})}
	case "backup status":
		return newBackupStatusData(&backup.StatusInfo{LastRun: "never", NextRun: "0 3 * * *", Health: "warning", TotalSize: "0B", RetentionDaily: 7, RetentionWeekly: 4, RetentionMonthly: 12},
			&backup.OffboxStatus{Source: "none", Problems: []string{}})
	}
	switch zero.(type) {
	case dataResult:
		return dataResult{OK: true, Target: ""}
	case dataHub:
		return dataHub{Subcommands: []dataHubEntry{{Name: "list", Summary: "List items"}}}
	}
	return emptyOf(reflect.TypeOf(zero)).Interface()
}

// emptyOf builds the zero value of t with every slice and map empty, not nil.
func emptyOf(t reflect.Type) reflect.Value {
	v := reflect.New(t).Elem()
	switch t.Kind() {
	case reflect.Slice:
		return reflect.MakeSlice(t, 0, 0)
	case reflect.Map:
		return reflect.MakeMap(t)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if f := v.Field(i); f.CanSet() && t.Field(i).Type.Kind() != reflect.Ptr && t.Field(i).Type.Kind() != reflect.Struct {
				f.Set(emptyOf(t.Field(i).Type))
			}
		}
	}
	return v
}
