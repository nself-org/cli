package httpapi

// Purpose: derive HTTP status from envelopes, rather than child exit status.
// Inputs: an invocation result. Outputs: status and response headers.
// Constraints: error codes take priority over classes; document bytes stay intact.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/output"
)

func status(r invoke.Result) int {
	var e struct {
		Error *struct{ Code, Class string }
		Data  json.RawMessage
	}
	_ = json.Unmarshal(r.Stdout, &e)
	code := r.ErrorCode
	if e.Error != nil {
		code = e.Error.Code
	}
	for _, row := range []struct {
		code   string
		status int
	}{
		{"E420", 400}, {"E425", 400}, {"E430", 400}, {"E426", 401}, {"E429", 403}, {"E421", 404}, {"E432", 404}, {"E424", 409}, {"E460", 409}, {"E428", 421}, {"E431", 503}, {"E423", 504},
	} {
		if row.code == code {
			return row.status
		}
	}
	if e.Error != nil {
		for _, row := range []struct {
			class  string
			status int
		}{{"user", 422}, {"auth", 403}, {"destructive_blocked", 409}, {"infra", 502}} {
			if row.class == e.Error.Class {
				return row.status
			}
		}
		return 500
	}
	if e.Data != nil {
		var d struct {
			ConfirmationRequired bool `json:"confirmation_required"`
		}
		_ = json.Unmarshal(e.Data, &d)
		if d.ConfirmationRequired {
			return 428
		}
		return 200
	}
	return 500
}

func document(w http.ResponseWriter, r invoke.Result, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Nself-Exit-Code", strconv.Itoa(r.ExitCode))
	w.Header().Set("X-Nself-Request-Id", r.RequestID)
	if code == 503 {
		w.Header().Set("Retry-After", "1")
	}
	w.WriteHeader(code)
	_, _ = w.Write(r.Stdout)
}

func (s *Server) failure(w http.ResponseWriter, r *http.Request, code string, httpCode int) {
	path := s.routes[r.URL.Path]
	e := errs.New(code, errs.Registry[code].Summary)
	var b bytes.Buffer
	_ = output.EmitError(output.Writer{Out: &b, Err: io.Discard}, path, e)
	id := invoke.RequestID(path, nil, invoke.Request{})
	document(w, invoke.Result{Stdout: b.Bytes(), ExitCode: errs.ExitCodeFor(e), RequestID: id, ErrorCode: code}, httpCode)
}
