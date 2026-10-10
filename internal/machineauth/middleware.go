package machineauth

// middleware.go — the bearer-token check for HTTP machine surfaces.
//
// Purpose: reject requests that do not carry the machine token.
// Inputs:  the expected token and the realm for WWW-Authenticate.
// Outputs: a handler wrapper answering 401 on a missing or wrong token.
// Constraints: header parsing is the v1.4 one (TrimPrefix of "Bearer "), so
//   behaviour with a token set is unchanged. Both sides are hashed before the
//   constant-time compare, so the check does not depend on the token length.
//   Nothing from the request is logged.

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
)

// Middleware returns a wrapper that serves the next handler only when the
// request presents "Authorization: Bearer <tok>". Otherwise it answers 401
// with the body "unauthorized" and a WWW-Authenticate: Bearer realm header. An
// empty tok rejects every request (fail closed).
func Middleware(tok, realm string) func(http.Handler) http.Handler {
	want := sha256.Sum256([]byte(tok))
	challenge := fmt.Sprintf("Bearer realm=%q", realm)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			sum := sha256.Sum256([]byte(got))
			if got == "" || tok == "" || subtle.ConstantTimeCompare(sum[:], want[:]) != 1 {
				w.Header().Set("WWW-Authenticate", challenge)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
