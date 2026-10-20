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

func TestReleaseAPIStatus(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			_, _, err := fetchReleaseFromURL(server.URL)
			var got *releaseAPIStatusError
			if !errors.As(err, &got) || got.StatusCode != status {
				t.Fatalf("lost HTTP status: %v", err)
			}
			coded := sourceFailure("release", fmt.Errorf("wrapped: %w", err))
			if errs.Describe(coded).Code != "E456" {
				t.Fatalf("wrong code: %v", coded)
			}
			if rateLimited := strings.Contains(errs.Describe(coded).Cause, "rate-limited"); rateLimited != (status == http.StatusTooManyRequests) {
				t.Fatalf("wrong status explanation: %v", coded)
			}
		})
	}
}
