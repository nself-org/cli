package commands

import (
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/output"
)

func statusReport(results ...health.HealthResult) *health.HealthReport {
	r := &health.HealthReport{Timestamp: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC), Results: results, Total: len(results)}
	for _, res := range results {
		if res.OK() {
			r.Healthy++
		} else {
			r.Unhealthy++
		}
	}
	return r
}

func TestPilotJSONStatus(t *testing.T) {
	up := health.HealthResult{Service: "postgres", Status: "healthy", Details: "ok"}
	starting := health.HealthResult{Service: "hasura", Status: "starting", Details: "starting"}
	down := health.HealthResult{Service: "auth", Status: "unhealthy", Details: "down"}

	t.Run("emit", func(t *testing.T) {
		compattest.Both(t, func(t *testing.T) {
			cases := []struct {
				name  string
				rep   *health.HealthReport
				state string
				v15   int
			}{
				{"healthy", statusReport(up), "ok", 0},
				{"starting", statusReport(up, starting), "transitional", 11},
				{"unhealthy", statusReport(up, starting, down), "unhealthy", 10},
			}
			for _, tc := range cases {
				out, _ := pilotBuffers(t)
				code, err := printStatusJSON(tc.rep)
				if err != nil {
					t.Fatal(err)
				}
				doc := singleDoc(t, out.String())
				if _, v15 := doc["schema_version"]; v15 {
					// v1.5: envelope with state, exit code from the state.
					data := validateEnvelope(t, out.String(), "status")
					if data["state"] != tc.state || code != tc.v15 {
						t.Fatalf("%s: state=%v code=%d, want %s/%d", tc.name, data["state"], code, tc.state, tc.v15)
					}
					continue
				}
				// v1.4: the bare pre-contract payload, no state, exit 0.
				if _, has := doc["state"]; has || code != 0 {
					t.Fatalf("%s: v1.4 payload has state or requests code %d: %s", tc.name, code, out)
				}
				if _, ok := doc["services"]; !ok {
					t.Fatalf("%s: v1.4 payload lacks services: %s", tc.name, out)
				}
			}
		})
	})

	t.Run("legacy env omits state", func(t *testing.T) {
		t.Setenv("NSELF_V15", "1")
		t.Setenv(output.LegacyEnvVar, "1")
		out, errOut := pilotBuffers(t)
		if _, err := printStatusJSON(statusReport(up)); err != nil {
			t.Fatal(err)
		}
		doc := singleDoc(t, out.String())
		if _, has := doc["state"]; has {
			t.Fatalf("legacy output carries state: %s", out)
		}
		if _, has := doc["schema_version"]; has || !strings.Contains(errOut.String(), "NSELF_JSON_LEGACY") {
			t.Fatalf("legacy output enveloped or warning missing: %s / %s", out, errOut)
		}
	})

	t.Run("live", func(t *testing.T) {
		skipLiveUnderV15Guard(t)
		proj := pilotProject(t)
		r := runPilot(t, proj, true, nil, "status", "--json")
		data := validateEnvelope(t, r.stdout, "status")
		if data["state"] != "unhealthy" || r.code != 10 {
			t.Fatalf("v1.5 status --json: state=%v exit=%d, want unhealthy/10\n%s", data["state"], r.code, r.stderr)
		}
		legacy := runPilot(t, proj, true, []string{"NSELF_JSON_LEGACY=1"}, "status", "--json")
		bare := singleDoc(t, legacy.stdout)
		if !equalJSON(t, withoutKeys(bare, "timestamp"), withoutKeys(data, "timestamp", "state")) {
			t.Fatalf("NSELF_JSON_LEGACY output differs from .data minus state:\n%s", legacy.stdout)
		}
		old := runPilot(t, proj, false, nil, "status", "--json")
		if _, has := singleDoc(t, old.stdout)["schema_version"]; has || old.code != 0 {
			t.Fatalf("v1.4 status --json must stay bare with exit 0, got exit %d\n%s", old.code, old.stdout)
		}
		human := runPilot(t, proj, true, nil, "status")
		if human.code != 10 {
			t.Fatalf("v1.5 human status exit = %d, want 10", human.code)
		}
		if h14 := runPilot(t, proj, false, nil, "status"); h14.code != 2 {
			t.Fatalf("v1.4 human status exit = %d, want 2", h14.code)
		}
	})
}
