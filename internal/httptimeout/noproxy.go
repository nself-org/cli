package httptimeout

import (
	"net/http"
	"time"
)

// NoProxy returns a fresh *http.Client for requests that carry a credential.
//
// Purpose: a bearer token must reach exactly the host the caller checked, so
// the client never consults HTTP_PROXY/HTTPS_PROXY (Transport.Proxy is nil) and
// never follows a redirect (a 3xx is returned to the caller as the response).
//
// Inputs: d is the whole-request timeout.
//
// Outputs: a client whose Transport is a clone of http.DefaultTransport with
// Proxy nil. Callers may type-assert Transport to *http.Transport to adjust
// the clone (for example a dial hook in tests).
//
// Constraints: use it only for token-bearing requests; ordinary traffic keeps
// the scoped clients above so proxy configuration still applies.
func NoProxy(d time.Duration) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	return &http.Client{
		Timeout:   d,
		Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
