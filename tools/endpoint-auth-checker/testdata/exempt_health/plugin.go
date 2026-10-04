// Exempt-health fixture — /health has no auth but is in ExemptRoutes.
package main

import (
	"net/http"
)

func registerRoutes(r interface{ Handle(string, ...interface{}) }) {
	// /health is in ExemptRoutes — should NOT trigger a violation.
	r.Handle("/health", handleHealth)
	// Protected route — compliant.
	r.Handle("/api/data", RequireUserJWT, handleData)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {}
func handleData(w http.ResponseWriter, r *http.Request)   {}

// RequireUserJWT is a stub middleware that satisfies the user JWT auth contract.
func RequireUserJWT(next http.Handler) http.Handler { return next }
