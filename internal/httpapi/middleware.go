package httpapi

// Purpose: reject unsafe HTTP requests before invocation or body reads.
// Inputs: HTTP request and server allowlists. Outputs: authenticated dispatch.
// Constraints: Host then Origin then Auth; never emit CORS headers.

import (
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// headerValue returns the first canonical header value, like Header.Get.
func headerValue(h http.Header, key string) string {
	values := h.Values(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (s *Server) authorized(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hosts[strings.ToLower(r.Host)] {
			s.failure(w, r, "E428", 421)
			return
		}
		origin := headerValue(r.Header, "Origin")
		_, originPresent := r.Header["Origin"]
		if len(r.Header.Values("Origin")) > 1 || originPresent && (!strings.HasPrefix(origin, "http://") || !s.hosts[strings.ToLower(strings.TrimPrefix(origin, "http://"))]) {
			s.failure(w, r, "E429", 403)
			return
		}
		if _, err := s.o.Auth.Authenticate(r); err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nself"`)
			s.failure(w, r, "E426", 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func accepts(value string) (stream, ok bool) {
	if value == "" {
		return false, true
	}
	for _, item := range strings.Split(value, ",") {
		typ, params, err := mime.ParseMediaType(strings.TrimSpace(item))
		if err != nil {
			continue
		}
		if value, exists := params["q"]; exists {
			q, err := strconv.ParseFloat(value, 64)
			if err != nil || !(q > 0 && q <= 1) {
				continue
			}
		}
		if typ == "application/x-ndjson" {
			return true, true
		}
		if typ == "application/json" || typ == "*/*" || typ == "application/*" {
			ok = true
		}
	}
	return false, ok
}
