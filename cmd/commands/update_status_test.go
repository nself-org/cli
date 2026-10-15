package commands

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestUpdateReleaseTypedStatus(t *testing.T) {
	for _, status := range []int{404, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()
			_, _, err := fetchReleaseFromURL(srv.URL)
			var got interface{ HTTPStatusCode() int }
			if !errors.As(fmt.Errorf("release lookup: %w", err), &got) || got.HTTPStatusCode() != status {
				t.Fatalf("release error %v does not carry HTTP status %d", err, status)
			}
			if status == 429 {
				d := errs.Describe(sourceFailure("release", fmt.Errorf("lookup: %w", err)))
				if d.Code != "E456" || !strings.Contains(d.Cause, "rate-limited") || d.ExitCode != 2 {
					t.Fatalf("rate limit description = %+v", d)
				}
			}
		})
	}
}

func TestUpdatePlanStatusTextIsNotHTTPStatus(t *testing.T) {
	for _, mode := range []string{"--plan", "--yes"} {
		t.Run(mode, func(t *testing.T) {
			dir := updateProject(t)
			before := hashTree(t, dir)
			r := newUpdateRecorder(t)
			r.cli = func() (string, string, error) {
				return "", "", errors.New("proxy diagnostic: status 404; status 429 is unrelated")
			}
			out, _, err := runUpdate(t, mode)
			if err == nil {
				t.Fatalf("error text was treated as HTTP status; output: %s", out)
			}
			d := errs.Describe(err)
			if d.Code != "E456" || strings.Contains(d.Cause, "rate-limited") {
				t.Fatalf("source failure description = %+v", d)
			}
			if hashTree(t, dir) != before {
				t.Fatal("source failure changed the project")
			}
		})
	}
}
