package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/plugin"
)

// stableDoctor drops what legitimately differs between two runs of doctor: the
// timestamp and the free-disk figures in the "Disk space" message and detail
// (the detail carries the used percent, which can tick between two runs).
func stableDoctor(doc map[string]any) map[string]any {
	out := withoutKeys(doc, "timestamp")
	checks, _ := out["checks"].([]any)
	stable := make([]any, 0, len(checks))
	for _, c := range checks {
		row, _ := c.(map[string]any)
		if strings.HasPrefix(row["name"].(string), "Disk space") {
			row = withoutKeys(row, "message", "detail")
		}
		stable = append(stable, row)
	}
	out["checks"] = stable
	return out
}

// equalJSON compares two decoded documents by their canonical encoding.
func equalJSON(t *testing.T, a, b map[string]any) bool {
	t.Helper()
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	return bytes.Equal(x, y)
}

// ---- doctor ----------------------------------------------------------------

func doctorFixture(statuses ...string) *doctorReport {
	var checks []doctorCheckResult
	for _, s := range statuses {
		checks = append(checks, doctorCheckResult{Name: "check " + s, Status: s, Message: "m"})
	}
	return buildDoctorReport(checks)
}

func TestPilotJSONDoctor(t *testing.T) {
	t.Run("emit", func(t *testing.T) {
		compattest.Both(t, func(t *testing.T) {
			cases := []struct {
				name  string
				rep   *doctorReport
				state string
				v15   int
			}{
				{"pass", doctorFixture("pass"), "ok", 0},
				{"warn", doctorFixture("pass", "warn"), "warnings", 12},
				{"fail", doctorFixture("pass", "warn", "fail"), "unhealthy", 10},
			}
			for _, tc := range cases {
				out, _ := pilotBuffers(t)
				err := printDoctorJSON(tc.rep)
				doc := singleDoc(t, out.String())
				if _, v15 := doc["schema_version"]; v15 {
					data := validateEnvelope(t, out.String(), "doctor")
					var ec *plugin.ExitCodeError
					got := 0
					if errors.As(err, &ec) {
						got = ec.Code
					} else if err != nil {
						t.Fatal(err)
					}
					if data["state"] != tc.state || got != tc.v15 {
						t.Fatalf("%s: state=%v exit=%d, want %s/%d", tc.name, data["state"], got, tc.state, tc.v15)
					}
					continue
				}
				// v1.4: bare report, state absent, --json exits 0.
				if _, has := doc["state"]; has || err != nil {
					t.Fatalf("%s: v1.4 doctor --json has state or err=%v", tc.name, err)
				}
			}
		})
	})

	t.Run("human exit codes", func(t *testing.T) {
		for _, tc := range []struct {
			rep      *doctorReport
			v14, v15 int
		}{
			{doctorFixture("pass"), 0, 0},
			{doctorFixture("warn"), 2, 12},
			{doctorFixture("warn", "fail"), 1, 10},
		} {
			for v15, want := range map[bool]int{false: tc.v14, true: tc.v15} {
				compattest.Set(t, v15)
				got := 0
				var ec *plugin.ExitCodeError
				if err := doctorExit(tc.rep); errors.As(err, &ec) {
					got = ec.Code
				}
				if got != want {
					t.Fatalf("doctorExit v15=%v = %d, want %d", v15, got, want)
				}
			}
		}
	})

	t.Run("live", func(t *testing.T) {
		skipLiveUnderV15Guard(t)
		proj := pilotProject(t)
		pinPilotDoctorSecret(t, proj)
		for _, args := range [][]string{{"doctor", "--json"}, {"doctor", "--format", "json"}, {"doctor", "--json", "--deep"}} {
			r := runPilot(t, proj, true, nil, args...)
			data := validateEnvelope(t, r.stdout, "doctor")
			if data["state"] != "unhealthy" || r.code != 10 {
				t.Fatalf("v1.5 %v: state=%v exit=%d, want unhealthy/10\n%s", args, data["state"], r.code, r.stderr)
			}
			if strings.Contains(r.stdout, pilotSecret) {
				t.Fatalf("%v printed the secret on stdout", args)
			}
		}
		env := runPilot(t, proj, true, nil, "doctor", "--json")
		legacy := runPilot(t, proj, true, []string{"NSELF_JSON_LEGACY=1"}, "doctor", "--json")
		want := stableDoctor(withoutKeys(singleDoc(t, env.stdout)["data"].(map[string]any), "state"))
		got := stableDoctor(singleDoc(t, legacy.stdout))
		if !equalJSON(t, got, want) {
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(want)
			t.Fatalf("NSELF_JSON_LEGACY output differs from .data minus state:\nlegacy %s\nwant   %s", a, b)
		}
		if h := runPilot(t, proj, true, nil, "doctor"); h.code != 10 {
			t.Fatalf("v1.5 human doctor exit = %d, want 10", h.code)
		}
		if h := runPilot(t, proj, false, nil, "doctor"); h.code != 1 {
			t.Fatalf("v1.4 human doctor exit = %d, want 1", h.code)
		}
		if old := runPilot(t, proj, false, nil, "doctor", "--json"); old.code != 0 || !strings.Contains(old.stdout, `"checks"`) {
			t.Fatalf("v1.4 doctor --json must exit 0 with the bare report, got exit %d", old.code)
		}
	})
}

// pinPilotDoctorSecret keeps password-strength results stable across processes.
func pinPilotDoctorSecret(t *testing.T, proj string) {
	t.Helper()
	path := filepath.Join(proj, ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("HASURA_GRAPHQL_ADMIN_SECRET=Q7vN2mZ8xP4rL6wY9bC3dF5hJ1kS0uAq\n")...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
