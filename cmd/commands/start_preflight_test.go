package commands

// Purpose: tests for the compose minimum-version refusal in nself start
// (checkComposeMinimum, D-0548) and its telemetry class.
// Inputs:  fake version strings and a fake error through the
//          dockerComposeVersion seam.
// Outputs: test failures only.
// Constraints: the init below stubs the seam for the whole package, so no test
//              in package commands runs docker. No subprocess runs here.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func init() {
	dockerComposeVersion = func(context.Context) (string, error) { return "2.29.7", nil }
}

func stubComposeVersion(t *testing.T, v string, err error) {
	t.Helper()
	orig := dockerComposeVersion
	t.Cleanup(func() { dockerComposeVersion = orig })
	dockerComposeVersion = func(context.Context) (string, error) { return v, err }
}

func TestCheckComposeMinimum(t *testing.T) {
	cases := []struct {
		name    string
		version string
		err     error
		refuse  bool
	}{
		{"too old refuses", "2.17.3", nil, true},
		{"minimum is ok", "2.20.0", nil, false},
		{"unrecognised fails open", "dev", nil, false},
		{"exec error fails open", "", errors.New("docker: not found"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stubComposeVersion(t, c.version, c.err)
			err := checkComposeMinimum(context.Background())
			if !c.refuse {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("want a refusal, got nil")
			}
			for _, want := range []string{"2.20", "backslash", "refuses"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err.Error(), want)
				}
			}
			if got := classifyStartError(err); got != "compose-too-old" {
				t.Errorf("classifyStartError = %q, want compose-too-old", got)
			}
		})
	}
}

func TestClassifyStartErrorOtherClassesUnchanged(t *testing.T) {
	cases := map[string]string{
		"bind: address already in use":     "port-collision",
		"cannot connect to the Docker":     "docker-not-running",
		"failed to pull image":             "image-pull-failed",
		"context deadline exceeded":        "healthcheck-timeout",
		"something unrelated went wrong..": "other",
	}
	for msg, want := range cases {
		if got := classifyStartError(errors.New(msg)); got != want {
			t.Errorf("classifyStartError(%q) = %q, want %q", msg, got, want)
		}
	}
}
