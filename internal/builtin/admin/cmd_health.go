package admin

import (
	"context"
	"fmt"
	"github.com/nself-org/cli/internal/httptimeout"
	"github.com/spf13/cobra"
	"net/http"
	"time"
)

func healthCommand(r runner) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check Admin service liveness",
		RunE:  r.runAdminHealth,
	}

}

func (r runner) runAdminHealth(cmd *cobra.Command, args []string) error {
	port := adminPort()
	url := "http://localhost:" + port + "/health"

	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	start := time.Now()
	resp, err := httptimeout.Default.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("Admin health: unhealthy (%v)\n", err)
		return fmt.Errorf("admin unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("Admin health: healthy (HTTP %d, %s)\n", resp.StatusCode, elapsed.Truncate(time.Millisecond))
		return nil
	}

	fmt.Printf("Admin health: unhealthy (HTTP %d, %s)\n", resp.StatusCode, elapsed.Truncate(time.Millisecond))
	return fmt.Errorf("admin returned HTTP %d", resp.StatusCode)
}
