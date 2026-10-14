package entitlement

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/nself-org/cli/internal/license"
)

// Purpose: record ping's error shape without changing shared validator policy.
// Inputs: HTTP request. Outputs: response/status. Constraints: bounded body read.
// SPORT: distinguish captive portals from entitlement refusals.
type validationStatus struct {
	client    license.HTTPDoer
	status    int
	pingError bool
}

func (s *validationStatus) Do(req *http.Request) (*http.Response, error) {
	resp, err := s.client.Do(req)
	if err != nil || resp == nil {
		return resp, err
	}
	s.status = resp.StatusCode
	if resp.StatusCode == http.StatusForbidden && resp.Body != nil {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		var shape struct {
			Valid *bool  `json:"valid"`
			Error string `json:"error"`
		}
		s.pingError = readErr == nil && len(body) <= 64*1024 && json.Unmarshal(body, &shape) == nil &&
			shape.Valid != nil && !*shape.Valid && shape.Error != ""
	}
	return resp, nil
}
