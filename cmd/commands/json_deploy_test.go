package commands

// Purpose: tests for the deploy fragment's v1 envelopes (P7-SURF-15): coverage
// of the canon rows, the golden writer, the v1.4 registry view of the rows
// that printed JSON before the contract, and the shared hub/result emission of
// the wrapped commands.
// Inputs: package-level commands driven in v1.5 mode with --json set.
// Constraints: no network, no remote host; hosts are the loopback fixtures.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	dprog "github.com/nself-org/cli/internal/deploy/progress"
	"github.com/nself-org/cli/internal/output"
)

// TestEnvelopeCoverageDeploy: every runnable document row of the deploy canon
// fragment is an envelope command.
func TestEnvelopeCoverageDeploy(t *testing.T) { assertFragmentEnvelopeCoverage(t, "deploy") }

// TestEnvelopeCoverageUpdateStack: the update_stack fragment (`update stack`).
func TestEnvelopeCoverageUpdateStack(t *testing.T) { assertFragmentEnvelopeCoverage(t, "update_stack") }

// TestDeployGoldenWrite regenerates the goldens of the deploy fragment; it
// runs only with NSELF_UPDATE_GOLDEN=1. `deploy promote` and `deploy targets`
// keep the goldens their own Tickets wrote.
func TestDeployGoldenWrite(t *testing.T) {
	if os.Getenv("NSELF_UPDATE_GOLDEN") != "1" {
		t.Skip("set NSELF_UPDATE_GOLDEN=1 to rewrite the deploy goldens")
	}
	for path, zero := range deployEnvelopeTypes {
		if path == "deploy promote" {
			continue
		}
		var buf bytes.Buffer
		if err := output.EmitData(output.Writer{Out: &buf, Err: &bytes.Buffer{}}, path, deployGoldenSample(path, zero)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixtureFileFor(path), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func deployGoldenSample(path string, zero any) any {
	switch path {
	case "deploy":
		return deployData{Target: "staging", Strategy: "rolling", Steps: []deployStep{{Name: "Build images", Status: "done"}},
			Servers: []deployServerRow{{Env: "staging", Server: "staging-app", Role: "app", Status: "ok"}}, DurationMS: 1200, Success: true}
	case "deploy status":
		return deployStatusData{Target: "staging", State: "running", Servers: []deployStatusServer{{Env: "staging", Server: "staging-app", Role: "app", Capability: "manage", LatencyMS: 12}}}
	case "deploy check-access":
		return deployCheckAccessData{Servers: []deployAccessRow{{Env: "staging", Server: "staging-app", Capability: "manage"}}, AllOK: true}
	case "deploy access list":
		return accessListData{Hosts: []accessHostRow{{Env: "staging", Server: "staging-app", Host: "u@staging.example.test", Entries: []accessListRow{
			{User: "alice", Fingerprint: "SHA256:fixture", Granted: "2026-10-01T00:00:00Z"}}, Status: "ok"}}}
	case "deploy environments":
		return deployEnvironmentsOutput{Environments: []envEnvironmentRow{{Name: "staging", Kind: "remote", Servers: []envServerRow{{Name: "staging-app", Role: "app", Capability: "manage"}}}}}
	case "deploy security setup":
		return securitySetupData{DryRun: true, Supported: true, Failed: []string{}, Steps: []securitySetupStep{{Name: "Allow SSH (22)", Command: []string{"ufw", "allow", "22/tcp"}}}}
	case "doctor security audit":
		return securityAuditData{Findings: []securityFinding{{Name: "firewall", OK: true, Detail: "ufw active"}}, Passed: 1}
	case "doctor security status":
		return securityStatusData{Findings: []securityFinding{{Name: "firewall", OK: true, Detail: "ufw active"}}, Score: 1, Total: 1, Percent: 100}
	case "update images":
		return updateImagesData{DryRun: true, Images: []updateImageRow{{Service: "postgres", Digest: strings.Repeat("a", 64), Status: "unchanged"}}, Failed: []string{}}
	case "update upgrade":
		return updateUpgradeData{OK: true, Action: "check", CurrentVersion: "1.5.0"}
	case "update verify-sbom":
		return verifySBOMData{Version: "v1.5.0", Repo: "nself-org/cli", Verified: true}
	case "deploy promote-env":
		// the alias answers with the engine result `deploy promote` already has a golden for
		var doc struct {
			Data cpromote.Result `json:"data"`
		}
		b, err := os.ReadFile(fixtureFileFor("deploy promote"))
		if err != nil || json.Unmarshal(b, &doc) != nil {
			panic("deploy promote golden is unreadable: run its Ticket's golden first")
		}
		return doc.Data
	}
	switch zero.(type) {
	case dataResult:
		return dataResult{OK: true}
	case dataHub:
		return dataHub{Subcommands: []dataHubEntry{{Name: "grant", Summary: "Grant a person SSH key access to a host"}}}
	}
	return emptyOf(reflect.TypeOf(zero)).Interface()
}

// deployV14JSONRows lists the rows that printed JSON in v1.4, as the v1.4
// registry path and the v1.5 key of the same row. The v1.4 view keeps them
// json: legacy; the v1.5 view reports envelope.
var deployV14JSONRows = map[string]string{
	"access list": "deploy access list", "deploy": "deploy", "deploy check-access": "deploy check-access",
	"deploy health": "deploy health", "deploy status": "deploy status", "promote": "deploy promote-env",
}

// TestDeployRegistryV14KeepsJSONLegacy: the v1.4 registry view describes v1.4
// behaviour: a row that printed JSON keeps json: legacy with no data schema,
// every other row of the fragment that exists in the v1.4 tree is none.
func TestDeployRegistryV14KeepsJSONLegacy(t *testing.T) {
	old, err := buildRegistry(false)
	if err != nil {
		t.Fatal(err)
	}
	cur, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	for v14, v15 := range deployV14JSONRows {
		row, ok := old.Lookup("nself " + v14)
		if !ok {
			t.Errorf("v1.4 registry has no row %q", v14)
			continue
		}
		if row.JSON != "legacy" || row.DataSchema != nil {
			t.Errorf("v1.4 %q: json=%q data_schema=%v, want legacy and no schema", v14, row.JSON, row.DataSchema)
		}
		nrow, ok := cur.Lookup("nself " + v15)
		if !ok || nrow.JSON != "envelope" || nrow.DataSchema == nil {
			t.Errorf("v1.5 %q: %+v, want envelope with a data schema", v15, nrow)
		}
	}
	legacy := map[string]bool{}
	for _, v15 := range deployV14JSONRows {
		legacy[v15] = true
	}
	checked := 0
	for key := range deployEnvelopeTypes {
		row, ok := old.Lookup("nself " + key)
		if legacy[key] || !ok {
			continue // printed JSON in v1.4, or moved in v1.5 (no row at this path in v1.4)
		}
		checked++
		if row.JSON == "envelope" || row.JSON == "legacy" {
			t.Errorf("v1.4 %q: json=%q, but no JSON is printed there in v1.4 (want none)", key, row.JSON)
		}
	}
	if checked < 4 {
		t.Errorf("only %d other deploy rows were checked in the v1.4 view; the check ran nothing", checked)
	}
}

// TestDeployConfirmBlocks: every remote or destructive row of the fragment
// declares a confirm block naming flags the command really has.
func TestDeployConfirmBlocks(t *testing.T) {
	reg, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"deploy": {"yes"}, "deploy access grant": {"yes"}, "deploy access revoke": {"yes"}, "deploy ops": {},
		"deploy promote": {"yes"}, "deploy promote-env": {"yes"}, "deploy promote-env rollback": {"yes"},
		"deploy rollback": {"yes"}, "deploy security setup": {"apply"}, "deploy web": {}, "update": {"yes"},
	}
	for path, flags := range want {
		row, ok := reg.Lookup("nself " + path)
		if !ok || row.Confirm == nil {
			t.Errorf("%s: no confirm block (row %v)", path, ok)
			continue
		}
		if strings.Join(row.Confirm.Flags, ",") != strings.Join(flags, ",") {
			t.Errorf("%s: confirm flags %v, want %v", path, row.Confirm.Flags, flags)
		}
	}
}

// jsonOnly drops everything before the first '{' (help text printed by the
// hub body shares the captured stream in these in-process tests).
func jsonOnly(out string) string {
	if i := strings.LastIndex(out, "{\n  \"schema_version\""); i >= 0 {
		return out[i:]
	}
	return out
}

// TestDeployStreamLinesUnchanged pins the exact bytes of the deploy progress stream, so
// moving the line writer onto output.StreamRecord and output.StreamEnd cannot change a
// byte on the wire. The expected lines were captured from the writer before the move.
func TestDeployStreamLinesUnchanged(t *testing.T) {
	clock := time.Unix(1700000000, 0)
	now := func() time.Time { clock = clock.Add(5 * time.Millisecond); return clock }
	var buf bytes.Buffer
	w := dprog.NewWriterClock(&buf, "deploy", now)
	w.At("prod", "").Event("Build images", dprog.PhaseStart, "")
	w.At("prod", "").Status("Build images", "done", "")
	w.At("prod", "web-1").Status("Deploy web-1", "failed", "ssh: token=abc123 refused <&>\nsecond line")
	w.Status("Deploy pipeline", "pending", "planned, not run (dry-run)")
	if err := w.Finish(dprog.ResultFailed, nil); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{"schema_version":"1","command":"deploy","data":{"type":"progress","seq":1,"step":"Build images","phase":"start","env":"prod","server":null,"elapsed_ms":5,"detail":null}}`,
		`{"schema_version":"1","command":"deploy","data":{"type":"progress","seq":2,"step":"Build images","phase":"done","env":"prod","server":null,"elapsed_ms":10,"detail":null}}`,
		`{"schema_version":"1","command":"deploy","data":{"type":"progress","seq":3,"step":"Deploy web-1","phase":"failed","env":"prod","server":"web-1","elapsed_ms":15,"detail":"ssh: token=[REDACTED] refused <&>"}}`,
		`{"schema_version":"1","command":"deploy","data":{"type":"progress","seq":4,"step":"Deploy pipeline","phase":"skipped","env":null,"server":null,"elapsed_ms":20,"detail":"planned, not run (dry-run)"}}`,
		`{"schema_version":"1","command":"deploy","data":{"type":"result","result":"failed","steps":[{"step":"Build images","status":"done"},{"step":"Deploy web-1","status":"failed"},{"step":"Deploy pipeline","status":"pending"}]}}`,
	}
	got := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), buf.String())
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d changed:\n got %s\nwant %s", i, got[i], want[i])
		}
	}

	// A broken sink stops the stream after the first failed write.
	bad := dprog.NewWriterClock(failWriter{}, "deploy", now)
	bad.Event("x", dprog.PhaseStart, "")
	if bad.Err() == nil {
		t.Fatal("write error was not recorded")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }
