// Compliant plugin fixture — all routes have recognized auth middleware.
package main

import (
	"net/http"
)

func registerRoutes(r interface{ Handle(string, ...interface{}) }) {
	// All routes wrapped with an allowlisted middleware.
	r.Handle("/api/data", RequireUserJWT, handleData)
	r.Handle("/api/admin", RequireHasuraAdminKey, handleAdmin)
	r.Handle("/api/license", RequireLicenseKey, handleLicense)
}

func handleData(w http.ResponseWriter, r *http.Request)    {}
func handleAdmin(w http.ResponseWriter, r *http.Request)   {}
func handleLicense(w http.ResponseWriter, r *http.Request) {}

// RequireUserJWT is a stub middleware that satisfies the user JWT auth contract.
func RequireUserJWT(next http.Handler) http.Handler { return next }

// RequireHasuraAdminKey is a stub middleware that satisfies the Hasura admin-key auth contract.
func RequireHasuraAdminKey(next http.Handler) http.Handler { return next }

// RequireLicenseKey is a stub middleware that satisfies the license-key auth contract.
func RequireLicenseKey(next http.Handler) http.Handler { return next }
