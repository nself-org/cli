package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestHealthyPassFixture(t *testing.T) {
	var out, errb bytes.Buffer
	code := dispatch([]string{"healthy", "-report", "testdata/golden-report-pass.json", "-json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	var res Result
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Metrics) != 1 || res.Metrics[0].Name != "time_to_healthy" || res.Metrics[0].Unit != "s" ||
		res.Metrics[0].P50 != 41 || res.Metrics[0].P95 != 41 || res.Metrics[0].Max != 41 || res.Metrics[0].N != 1 {
		t.Errorf("got %+v", res.Metrics)
	}
}

func TestHealthyFailsOnBadStep(t *testing.T) {
	var out, errb bytes.Buffer
	code := dispatch([]string{"healthy", "-report", "testdata/golden-report-step5-fail.json"}, &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), `step 5 status "fail"`) {
		t.Errorf("exit %d stderr %q", code, errb.String())
	}
}

func TestTimeToHealthyCases(t *testing.T) {
	cases := []struct {
		name, doc string
		want      float64
		wantErr   string
	}{
		{"warn counts", `{"steps":{"3":{"status":"pass","duration":1},"4":{"status":"warn","duration":2},"5":{"status":"pass","duration":3},"6":{"status":"pass","duration":4}}}`, 10, ""},
		{"missing step", `{"steps":{"3":{"status":"pass","duration":1}}}`, 0, "step 4 missing"},
		{"skipped step", `{"steps":{"3":{"status":"pass"},"4":{"status":"pass"},"5":{"status":"skipped"},"6":{"status":"pass"}}}`, 0, `step 5 status "skipped"`},
		{"not json", `nope`, 0, "parse report"},
		{"no steps", `{}`, 0, "step 3 missing"},
	}
	for _, c := range cases {
		got, err := timeToHealthy([]byte(c.doc))
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("%s: err %v, want %q", c.name, err, c.wantErr)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %v, %v want %v", c.name, got, err, c.want)
		}
	}
}

func TestHealthyUsageErrors(t *testing.T) {
	var out, errb bytes.Buffer
	if code := dispatch([]string{"healthy"}, &out, &errb); code != 2 {
		t.Errorf("no -report: exit %d", code)
	}
	if code := dispatch([]string{"healthy", "-report", "testdata/missing.json"}, &out, &errb); code != 2 {
		t.Errorf("missing file: exit %d", code)
	}
}
