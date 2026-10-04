// Noncompliant plugin fixture — one route is missing auth middleware.
package main

import (
	"net/http"
)

func registerRoutes(r interface{ Handle(string, ...interface{}) }) {
	// This route is properly protected.
	r.Handle("/api/safe", RequireUserJWT, handleSafe)
	// This route has NO auth middleware — should trigger a violation.
	r.Handle("/api/leak", handleLeak)
}

func handleSafe(w http.ResponseWriter, r *http.Request) {}
func handleLeak(w http.ResponseWriter, r *http.Request) {}

// RequireUserJWT is a stub middleware that satisfies the user JWT auth contract.
func RequireUserJWT(next http.Handler) http.Handler { return next }
