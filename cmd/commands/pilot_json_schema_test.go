package commands

import (
	"encoding/json"
	"testing"
)

// pilotDataDoc marshals a data value to the generic form the schema tests edit.
func pilotDataDoc(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestPilotSchemaEnumsAndArrays: the generated data schemas accept real data
// and reject a value outside a closed string set and a null array.
func TestPilotSchemaEnumsAndArrays(t *testing.T) {
	status := pilotDataDoc(t, statusData{statusJSONOutput: statusJSONOutput{
		Timestamp: "2026-10-05T12:00:00Z",
		Services:  []statusJSONService{{Name: "postgres", Status: "healthy", Duration: "1ms", Details: "ok"}},
	}, State: stateOK})
	doctor := pilotDataDoc(t, doctorData{doctorReport: *doctorFixture("pass", "warn"), State: stateWarnings})
	list := pilotDataDoc(t, configListData{File: ".env", Keys: configListRows(map[string]string{"PROJECT_NAME": "x"})})

	cases := []struct {
		cmd  string
		good map[string]any
		bad  map[string]func(m map[string]any)
	}{
		{"status", status, map[string]func(map[string]any){
			"bad state":      func(m map[string]any) { m["state"] = "banana" },
			"warnings state": func(m map[string]any) { m["state"] = stateWarnings }, // doctor-only value
			"null services":  func(m map[string]any) { m["services"] = nil },
		}},
		{"doctor", doctor, map[string]func(map[string]any){
			"bad state":    func(m map[string]any) { m["state"] = "banana" },
			"transitional": func(m map[string]any) { m["state"] = stateTransitional }, // status-only value
			"null checks":  func(m map[string]any) { m["checks"] = nil },
		}},
		{"config list", list, map[string]func(map[string]any){
			"bad source": func(m map[string]any) { m["keys"].([]any)[0].(map[string]any)["source"] = "weird" },
			"null keys":  func(m map[string]any) { m["keys"] = nil },
		}},
	}
	for _, tc := range cases {
		rs := contractResolve(t, schemaFileFor(tc.cmd))
		raw, _ := json.Marshal(tc.good)
		if err := contractValidate(rs, raw); err != nil {
			t.Fatalf("%s: real data rejected: %v\n%s", tc.cmd, err, raw)
		}
		for name, mutate := range tc.bad {
			m := pilotDataDoc(t, tc.good) // fresh copy per mutation
			mutate(m)
			raw, _ := json.Marshal(m)
			if err := contractValidate(rs, raw); err == nil {
				t.Errorf("%s: %s accepted by the schema: %s", tc.cmd, name, raw)
			}
		}
	}
}

// TestPilotJSONUnderV15Guard runs the JSON tests of the pilots (and the doctor
// JSON unit tests that predate them) with NSELF_V15=1 set through t.Setenv, so
// the default CI run, which never sets the variable, covers v1.5 mode: a
// pilot change that only breaks under the flag fails here, not at the flip.
func TestPilotJSONUnderV15Guard(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	for _, tc := range []struct {
		name string
		fn   func(*testing.T)
	}{
		{"PrintDoctorJSON_Valid", TestPrintDoctorJSON_Valid},
		{"PrintDoctorJSON_EmptyChecks", TestPrintDoctorJSON_EmptyChecks},
		{"PilotJSONStatus", TestPilotJSONStatus},
		{"PilotJSONDoctor", TestPilotJSONDoctor},
		{"PilotJSONConfig", TestPilotJSONConfig},
		{"PilotSchemaEnumsAndArrays", TestPilotSchemaEnumsAndArrays},
	} {
		t.Run(tc.name, tc.fn)
	}
}
