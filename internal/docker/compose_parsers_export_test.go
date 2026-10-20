package docker_test

import (
	"testing"

	"github.com/nself-org/cli/internal/docker"
)

func TestExportedComposeParsers(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		bad       bool
		want      int
	}{
		{"empty", "", false, 0},
		{"array", `[{"ID":"a","State":"running"}]`, false, 1},
		{"lines", `{"ID":"a"}
{"ID":"b"}
`, false, 2},
		{"invalid", "{", true, 0},
	} {
		t.Run("states/"+tc.name, func(t *testing.T) {
			got, err := docker.ParseContainerStates([]byte(tc.raw))
			if (err != nil) != tc.bad {
				t.Fatalf("error = %v, want failure %v", err, tc.bad)
			}
			if !tc.bad && len(got) != tc.want {
				t.Fatalf("states = %+v, want %d rows", got, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name, raw   string
		bad, health bool
	}{
		{"enabled", `{"services":{"web":{"healthcheck":{"test":["CMD","true"]}}}}`, false, true},
		{"none", `{"services":{"web":{"healthcheck":{"test":["NONE"]}}}}`, false, false},
		{"disabled", `{"services":{"web":{"healthcheck":{"disable":true}}}}`, false, false},
		{"invalid", "{", true, false},
	} {
		t.Run("specs/"+tc.name, func(t *testing.T) {
			got, err := docker.ParseServiceSpecs([]byte(tc.raw))
			if (err != nil) != tc.bad {
				t.Fatalf("error = %v, want failure %v", err, tc.bad)
			}
			if !tc.bad && (len(got) != 1 || got[0].HasHealthcheck != tc.health) {
				t.Fatalf("specs = %+v", got)
			}
		})
	}
}
