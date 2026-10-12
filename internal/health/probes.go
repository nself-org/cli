package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nself-org/cli/internal/httptimeout"
)

const probeTimeout = 5 * time.Second

// ProbeMinIO performs an HTTP GET to /minio/health/live on the given host:port.
// MinIO returns HTTP 200 when healthy.
func ProbeMinIO(ctx context.Context, host string, port int) *HealthResult {
	url := fmt.Sprintf("http://%s:%d/minio/health/live", host, port)
	return probeHTTP(ctx, "minio", url, func(statusCode int, _ []byte) (string, string) {
		if statusCode == http.StatusOK {
			return "healthy", fmt.Sprintf("HTTP %d", statusCode)
		}
		return "unhealthy", fmt.Sprintf("HTTP %d (expected 200)", statusCode)
	})
}

// ProbeMeiliSearch performs an HTTP GET to /health on the given host:port.
// MeiliSearch returns {"status":"available"} when healthy.
func ProbeMeiliSearch(ctx context.Context, host string, port int) *HealthResult {
	url := fmt.Sprintf("http://%s:%d/health", host, port)
	return probeHTTP(ctx, "search", url, func(statusCode int, body []byte) (string, string) {
		if statusCode != http.StatusOK {
			return "unhealthy", fmt.Sprintf("HTTP %d (expected 200)", statusCode)
		}
		var payload struct {
			Status  string `json:"status"`
			Version string `json:"pkgVersion"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return "unhealthy", fmt.Sprintf("bad JSON: %v", err)
		}
		if payload.Status == "available" {
			detail := "status=available"
			if payload.Version != "" {
				detail += fmt.Sprintf(" version=%s", payload.Version)
			}
			return "healthy", detail
		}
		return "unhealthy", fmt.Sprintf("status=%q (expected \"available\")", payload.Status)
	})
}

// ProbeNginxHTTP performs an HTTP GET on port 80 of the given host and
// considers any HTTP response (including redirects) as healthy.
func ProbeNginxHTTP(ctx context.Context, host string, port int) *HealthResult {
	url := fmt.Sprintf("http://%s:%d/", host, port)
	ctx2, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	start := time.Now()

	// Use a client that does NOT follow redirects — we just want any HTTP response.
	client := &http.Client{
		Timeout: probeTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx2, http.MethodGet, url, nil)
	if err != nil {
		return &HealthResult{
			Service:  "nginx",
			Status:   "unhealthy",
			Duration: time.Since(start),
			Details:  fmt.Sprintf("request build error: %v", err),
		}
	}

	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return &HealthResult{
			Service:  "nginx",
			Status:   "unhealthy",
			Duration: elapsed,
			Details:  err.Error(),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	// Any HTTP response (200, 301, 302, 403...) means Nginx is up.
	return &HealthResult{
		Service:  "nginx",
		Status:   "healthy",
		Duration: elapsed,
		Details:  fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}

// ProbeMailpit performs an HTTP GET to /api/v1/info on the given host:port.
// Mailpit returns {"Version":"..."} when healthy.
func ProbeMailpit(ctx context.Context, host string, port int) *HealthResult {
	url := fmt.Sprintf("http://%s:%d/api/v1/info", host, port)
	return probeHTTP(ctx, "email", url, func(statusCode int, body []byte) (string, string) {
		if statusCode != http.StatusOK {
			return "unhealthy", fmt.Sprintf("HTTP %d (expected 200)", statusCode)
		}
		var payload struct {
			Version string `json:"Version"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return "unhealthy", fmt.Sprintf("bad JSON: %v", err)
		}
		if payload.Version != "" {
			return "healthy", fmt.Sprintf("Mailpit version=%s", payload.Version)
		}
		return "healthy", "HTTP 200"
	})
}

// probeHTTP is a generic HTTP probe helper. It calls the given URL, reads the
// body, and delegates status/detail determination to the provided callback.
func probeHTTP(ctx context.Context, service, url string, decide func(int, []byte) (string, string)) *HealthResult {
	ctx2, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx2, http.MethodGet, url, nil)
	if err != nil {
		return &HealthResult{
			Service:  service,
			Status:   "unhealthy",
			Duration: time.Since(start),
			Details:  fmt.Sprintf("request build error: %v", err),
		}
	}

	resp, err := httptimeout.Health.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return &HealthResult{
			Service:  service,
			Status:   "unhealthy",
			Duration: elapsed,
			Details:  err.Error(),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	status, detail := decide(resp.StatusCode, body)
	return &HealthResult{
		Service:  service,
		Status:   status,
		Duration: elapsed,
		Details:  detail,
	}
}
