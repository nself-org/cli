package httpapi

// Purpose: flush child NDJSON bytes and publish its exit status as a trailer.
// Inputs: a request context and invoker. Outputs: streaming response.
// Constraints: cancellation kills the child; pre-stream refusals remain JSON.

import (
	"net/http"
	"strconv"
	"time"

	"github.com/nself-org/cli/internal/invoke"
)

type flushWriter struct {
	w       http.ResponseWriter
	started bool
	id      string
}

func (f *flushWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if !f.started {
		f.w.Header().Set("Content-Type", "application/x-ndjson")
		f.w.Header().Set("Trailer", "X-Nself-Exit-Code")
		f.w.Header().Set("X-Nself-Request-Id", f.id)
		f.w.WriteHeader(200)
		f.started = true
	}
	n, err := f.w.Write(p)
	if fl, ok := f.w.(http.Flusher); ok {
		fl.Flush()
	}
	return n, err
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request, path string, req invoke.Request, o invoke.Options) {
	cmd, _ := s.o.Registry.Lookup(path)
	f := &flushWriter{w: w, id: invoke.RequestID(path, cmd, req)}
	o.Transport, o.Timeout, o.Stdout, o.KillDelay = invoke.TransportHTTPStream, 0, f, 1500*time.Millisecond
	res, err := s.o.Invoker(r.Context(), s.o.Registry, path, req, o)
	if err != nil {
		s.logInvokeError(r, err)
	}
	if f.started {
		exitCode := "-1"
		if err == nil {
			exitCode = strconv.Itoa(res.ExitCode)
		}
		w.Header().Set("X-Nself-Exit-Code", exitCode)
		return
	}
	if err != nil {
		if r.Context().Err() == nil {
			s.failure(w, r, "E422", 502)
		}
		return
	}
	if len(res.Stdout) == 0 {
		s.failure(w, r, "E422", 502)
		return
	}
	document(w, res, status(res))
}
