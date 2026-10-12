package commands

// Purpose: tests for checkDockerComposeVersion (D-0548) with the version read
// stubbed through the dockerComposeVersion seam.
// Inputs:  fake version strings and a fake error.
// Outputs: test failures only.
// Constraints: no subprocess runs.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCheckDockerComposeVersion(t *testing.T) {
	cases := []struct {
		name       string
		version    string
		err        error
		wantStatus string
		wantMsg    string
	}{
		{"too old", "2.17.3", nil, "warn", "older than 2.20"},
		{"new enough with v prefix", "v2.29.7", nil, "pass", "docker compose v2.29.7"},
		{"unrecognised", "dev", nil, "pass", "not recognised"},
		{"exec error", "", errors.New("exit status 1"), "fail", "not available"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			orig := dockerComposeVersion
			t.Cleanup(func() { dockerComposeVersion = orig })
			dockerComposeVersion = func(context.Context) (string, error) { return c.version, c.err }
			got := checkDockerComposeVersion(context.Background(), false)
			if got.Status != c.wantStatus {
				t.Errorf("status = %q, want %q (%+v)", got.Status, c.wantStatus, got)
			}
			if !strings.Contains(got.Message, c.wantMsg) {
				t.Errorf("message %q lacks %q", got.Message, c.wantMsg)
			}
			if c.wantStatus == "warn" && !strings.Contains(got.Message, "backslash") {
				t.Errorf("warn message must say why: %q", got.Message)
			}
		})
	}
}
