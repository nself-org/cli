package httpapi

// Purpose: route registry command paths to the shared machine invoker.
// Inputs: authenticated requests. Outputs: original document or streamed bytes.
// Constraints: strict bounded request decoding; in-flight limit follows validation.

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/nself-org/cli/internal/invoke"
)

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/openapi.json" {
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			s.failure(w, r, "E430", 405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(s.openapi)
		return
	}
	path, known := s.routes[r.URL.Path]
	if !known {
		s.failure(w, r, "E432", 404)
		return
	}
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		s.failure(w, r, "E430", 405)
		return
	}
	typ, _, err := mime.ParseMediaType(headerValue(r.Header, "Content-Type"))
	if err != nil || typ != "application/json" {
		s.failure(w, r, "E430", 415)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, invoke.MaxRequestBytes))
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			s.failure(w, r, "E430", 413)
		} else {
			s.failure(w, r, "E430", 400)
		}
		return
	}
	if len(data) == 0 || !json.Valid(data) {
		s.failure(w, r, "E430", 400)
		return
	}
	req, err := invoke.DecodeRequest(data)
	if err != nil {
		s.failure(w, r, "E420", 400)
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
	stream, ok := accepts(headerValue(r.Header, "Accept"))
	if !ok {
		s.failure(w, r, "E430", 406)
		return
	}
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	default:
		s.failure(w, r, "E431", 503)
		return
	}
	o := invoke.Options{Transport: invoke.TransportHTTP, Surface: "http", Guard: s.guard, Dir: s.o.ProjectDir, Timeout: s.o.Timeout}
	if stream {
		s.stream(w, r, path, req, o)
		return
	}
	res, err := s.o.Invoker(r.Context(), s.o.Registry, path, req, o)
	if err != nil {
		if r.Context().Err() == nil {
			s.failure(w, r, "E422", 502)
		}
		return
	}
	document(w, res, status(res))
}
