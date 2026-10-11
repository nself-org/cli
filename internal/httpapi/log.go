package httpapi

// Purpose: log one bounded, token-free line per HTTP request.
// Inputs: method, URL path, elapsed time. Outputs: stderr or injected log writer.
// Constraints: never log queries, headers or bodies; preserve streaming Flush.

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type responseLog struct {
	http.ResponseWriter
	status int
}

func (w *responseLog) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *responseLog) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *responseLog) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(p)
}
func (w *responseLog) Flush() {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (s *Server) logged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		out := &responseLog{ResponseWriter: w}
		next.ServeHTTP(out, r)
		path := r.URL.Path
		if len(path) > 200 {
			path = path[:200]
		}
		if out.status == 0 {
			out.status = 200
		}
		s.logMu.Lock()
		defer s.logMu.Unlock()
		_, _ = fmt.Fprintf(s.o.Log, "%s %s %d %dms\n", strconv.Quote(r.Method), strconv.Quote(path), out.status, time.Since(start).Milliseconds())
	})
}
