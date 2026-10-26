package commands

// Purpose: Regression coverage for P6 FIX-CLI-2 (found 2026-09-03) — `nself
// start`'s "Starting PostgreSQL" step ran `docker compose up -d postgres`
// unconditionally on every invocation, even when a healthy postgres
// container for the project was already running. Compose's own config-hash
// diff normally makes that a no-op, but an on-disk docker-compose.yml that
// drifted from what actually created the running container (regenerated
// elsewhere, .env edited since the last `nself build`, etc.) makes Compose
// treat it as a config change and attempt to recreate an already-healthy
// postgres — which can fail mid-recreate. `nself start` run a second time
// against a live, healthy stack hit exactly this; `docker compose up` (no
// service filter) succeeded immediately after as a manual workaround.
// startPostgresPhase (start_containers.go) now checks
// docker.GetHealthStatus for the project's postgres container before
// calling ComposeUp, and skips the compose-up call entirely when it is
// already "healthy". postgresAlreadyRunning is the pure decision predicate
// behind that guard.
// Inputs: none (pure string-in/bool-out).
// Outputs: pass/fail via testing.T.
// Constraints: table-driven per every internal/docker.GetHealthStatus return
//              value ("healthy", "unhealthy", "starting", "none",
//              "not_found") plus an empty string for the zero value.

import (
	"strings"
	"testing"
)

func TestPostgresAlreadyRunning(t *testing.T) {
	tests := []struct {
		name         string
		healthStatus string
		want         bool
	}{
		{"healthy container is treated as already running", "healthy", true},
		{"unhealthy container still needs compose up", "unhealthy", false},
		{"starting container has not passed its first probe yet", "starting", false},
		{"none (no healthcheck configured) is not a positive signal", "none", false},
		{"not_found (container does not exist) needs compose up", "not_found", false},
		{"empty string (zero value / lookup error) needs compose up", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := postgresAlreadyRunning(tt.healthStatus); got != tt.want {
				t.Errorf("postgresAlreadyRunning(%q) = %v, want %v", tt.healthStatus, got, tt.want)
			}
		})
	}
}

// D-0708: .env.secrets wins over .env, so the credential hint must name it.
func TestDBInitCredentialHints_NameEnvSecrets(t *testing.T) {
	joined := strings.Join(dbInitCredentialHints, "\n")
	if !strings.Contains(joined, ".env.secrets") {
		t.Errorf("hints must point at .env.secrets, got:\n%s", joined)
	}
}
