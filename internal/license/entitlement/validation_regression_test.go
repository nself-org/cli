package entitlement

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/license"
)

type statusDoer func(*http.Request) (*http.Response, error)

func (f statusDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }

type failingStatusBody struct{ io.Reader }

func (b failingStatusBody) Read(p []byte) (int, error) {
	n, _ := b.Reader.Read(p)
	return n, errors.New("incomplete response")
}

func TestValidationStatusBoundsAndAuthenticatesErrorShape(t *testing.T) {
	shape := `{"valid":false,"error":"rejected"}`
	for _, tc := range []struct {
		name string
		body io.Reader
		want bool
		size int
	}{
		{"ping", strings.NewReader(shape), true, len(shape)},
		{"exact-limit", strings.NewReader(shape + strings.Repeat(" ", 64*1024-len(shape))), true, 64 * 1024},
		{"over-limit", strings.NewReader(shape + strings.Repeat(" ", 64*1024+100-len(shape))), false, 64*1024 + 1},
		{"read-error", failingStatusBody{strings.NewReader(shape)}, false, len(shape)},
		{"valid-response", strings.NewReader(`{"valid":true,"error":"notice"}`), false, 31},
		{"missing-valid", strings.NewReader(`{"error":"rejected"}`), false, 20},
		{"missing-error", strings.NewReader(`{"valid":false}`), false, 15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := &validationStatus{client: statusDoer(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(tc.body)}, nil
			})}
			resp, err := remote.Do(&http.Request{})
			if err != nil || remote.pingError != tc.want || remote.status != http.StatusForbidden {
				t.Fatalf("pingError=%v, status=%d, err=%v", remote.pingError, remote.status, err)
			}
			replayed, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "over-limit" && len(replayed) != tc.size {
				t.Fatalf("bounded replay length=%d, want %d", len(replayed), tc.size)
			}
			if tc.want && !bytes.HasPrefix(replayed, []byte(shape)) {
				t.Fatal("validator lost the response body")
			}
		})
	}
}

func TestExplicitRejectionRequiresOnlineRefusal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remote validationStatus
		result *license.ValidatorResult
		want   bool
	}{
		{"unauthorized", validationStatus{status: 401}, nil, true},
		{"portal", validationStatus{status: 403}, nil, false},
		{"ping", validationStatus{status: 403, pingError: true}, nil, true},
		{"signed-revoked", validationStatus{status: 200}, &license.ValidatorResult{Status: license.StatusRevoked}, true},
		{"signed-expired", validationStatus{status: 200}, &license.ValidatorResult{Status: license.StatusExpired}, true},
		{"cached-revoked", validationStatus{status: 200}, &license.ValidatorResult{Status: license.StatusRevoked, FromCache: true}, false},
		{"valid", validationStatus{status: 200}, &license.ValidatorResult{Status: license.StatusValid}, false},
		{"no-result", validationStatus{status: 200}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := explicitRejection(&tc.remote, tc.result); got != tc.want {
				t.Fatalf("rejection=%v, want %v", got, tc.want)
			}
		})
	}
}
