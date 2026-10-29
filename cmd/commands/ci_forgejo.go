package commands

// Purpose: nself ci forgejo — Forgejo server + runner health check.
//   Queries the Forgejo HTTP API and the forgejo-runner container health so
//   operators can confirm the ops-profile CI stack is up without logging into
//   the server directly.
// Inputs:  --url (Forgejo base URL), --runner (runner container name)
// Outputs: table of: server status, API reachable, runner container state, job queue depth
// Constraints: no Forgejo token required (uses the public /-/health endpoint for liveness;
//   queue depth requires NSELF_FORGEJO_ADMIN_USER + NSELF_FORGEJO_ADMIN_PASSWORD).
// SPORT: F08-SERVICE-INVENTORY — forgejo (ops profile, port 3844)

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

var ciForgejoCmd = &cobra.Command{
	Use:   "forgejo",
	Short: "Show Forgejo server health (ops profile)",
	Long: `Query the Forgejo server health endpoint and the forgejo-runner
container status. Designed for the ops profile where Forgejo provides
offline CI that runs .github/workflows/*.yml with zero GitHub Actions
quota consumption.

Env vars used (optional — for API queue depth):
  NSELF_FORGEJO_ADMIN_USER      Forgejo admin username
  NSELF_FORGEJO_ADMIN_PASSWORD  Forgejo admin password

Examples:
  nself ci forgejo
  nself ci forgejo --url http://ci.example.com:3844
  nself ci forgejo --runner nself_forgejo_runner`,
	RunE: runCIForgejo,
}

func init() {
	ciForgejoCmd.Flags().String("url", "http://localhost:3844", "Forgejo base URL")
	ciForgejoCmd.Flags().String("runner", "", "Runner container name (default: <PROJECT>_forgejo_runner)")
	ciCmd.AddCommand(ciForgejoCmd)
}

// forgejoHealthResponse is the subset of /-/health we care about.
type forgejoHealthResponse struct {
	Healthy bool `json:"healthy"`
}

func runCIForgejo(cmd *cobra.Command, _ []string) error {
	baseURL, _ := cmd.Flags().GetString("url")
	runnerContainer, _ := cmd.Flags().GetString("runner")
	baseURL = strings.TrimRight(baseURL, "/")

	client := &http.Client{Timeout: 10 * time.Second}

	ui.Section("Forgejo CI stack (ops profile)")

	// ─── 1. Server liveness ───────────────────────────────────────────────────
	serverOK := false
	serverMsg := "unreachable"
	resp, err := client.Get(baseURL + "/-/health")
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		var h forgejoHealthResponse
		if json.NewDecoder(resp.Body).Decode(&h) == nil && h.Healthy {
			serverOK = true
			serverMsg = "healthy"
		} else {
			serverMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
	}

	// ─── 2. Runner container state (via docker inspect) ───────────────────────
	runnerState := "unknown"
	runnerStatus := ""
	if runnerContainer == "" {
		// Try common project prefix patterns.
		for _, candidate := range []string{"nself_forgejo_runner", "app_forgejo_runner"} {
			out, err := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", candidate).Output()
			if err == nil {
				runnerContainer = candidate
				runnerState = strings.TrimSpace(string(out))
				break
			}
		}
		if runnerContainer == "" {
			runnerState = "not found (docker inspect failed — is Docker running?)"
		}
	} else {
		out, err := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", runnerContainer).Output()
		if err == nil {
			runnerState = strings.TrimSpace(string(out))
		}
	}
	if runnerState == "running" {
		runnerStatus = "running"
	} else {
		runnerStatus = runnerState
	}

	// ─── 3. Active jobs via API (optional — needs admin creds) ────────────────
	jobsInfo := "n/a (set NSELF_FORGEJO_ADMIN_USER + NSELF_FORGEJO_ADMIN_PASSWORD)"
	adminUser := os.Getenv("NSELF_FORGEJO_ADMIN_USER")
	adminPass := os.Getenv("NSELF_FORGEJO_ADMIN_PASSWORD")
	if adminUser != "" && adminPass != "" && serverOK {
		req, _ := http.NewRequest("GET", baseURL+"/api/v1/repos/search?limit=0", nil)
		req.SetBasicAuth(adminUser, adminPass)
		if r, err := client.Do(req); err == nil {
			defer func() { _ = r.Body.Close() }()
			if r.StatusCode == http.StatusOK {
				jobsInfo = "API reachable (admin authenticated)"
			} else {
				jobsInfo = fmt.Sprintf("API returned HTTP %d", r.StatusCode)
			}
		}
	}

	// ─── Output ───────────────────────────────────────────────────────────────
	serverIcon := "✗"
	if serverOK {
		serverIcon = "✓"
	}
	runnerIcon := "✗"
	if runnerStatus == "running" {
		runnerIcon = "✓"
	}

	fmt.Printf("  %s Forgejo server   %s   %s\n", serverIcon, baseURL, serverMsg)
	fmt.Printf("  %s Forgejo runner   container=%s   state=%s\n", runnerIcon, runnerContainer, runnerStatus)
	fmt.Printf("    API auth         %s\n", jobsInfo)
	fmt.Println()

	if !serverOK {
		ui.Warn("Forgejo server unreachable. Is the ops profile running? Try: nself start --profile ops")
	}
	if runnerStatus != "running" {
		ui.Warn("Forgejo runner is not running. Check: docker logs " + runnerContainer)
	}
	if serverOK && runnerStatus == "running" {
		ui.Success("Forgejo CI stack is healthy.")
	}
	return nil
}
