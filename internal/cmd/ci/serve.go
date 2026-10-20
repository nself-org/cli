// Package ci — serve.go
//
// Purpose: HTTP webhook server for nself ci serve. Listens for GitHub push and
//
//	pull_request events, verifies HMAC-SHA256 signatures, and dispatches gate
//	jobs to a bounded worker pool. Each job runs in an ephemeral Docker
//	container so a runaway build cannot starve the host.
//
// Inputs:  ServeConfig (addr, secret, concurrency, workdir, timeout)
// Outputs: HTTP server; /healthz 200 OK; / info page; GitHub commit status per job
// Constraints: stdlib only (no external HTTP frameworks); Docker + gh on PATH;
//
//	graceful shutdown on SIGINT/SIGTERM; SPORT CLI-CMD-CI-SERVE-001
package ci

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nself-org/cli/internal/ui"
)

// ServeConfig holds all runtime parameters for the webhook server.
type ServeConfig struct {
	Addr        string // listen address, e.g. ":3845"
	Secret      string // HMAC secret; falls back to GITHUB_WEBHOOK_SECRET env
	Concurrency int    // max concurrent jobs
	WorkDir     string // base dir for ephemeral checkouts
	JobTimeout  int    // per-job timeout in seconds
	Verbose     bool

	// ResolveGate returns the path of the nself-ci gate binary. The cmd layer
	// supplies its verified resolver (no PATH lookup, no go install); there is no
	// fallback here, so a nil value is a refusal to start.
	ResolveGate func(verbose bool) (string, error)

	// Insecure opts out of the fail-closed "refuse to start without a
	// webhook secret" default (T32). When true and no secret is configured,
	// RunServe starts anyway with signature verification disabled — the
	// previous, unconditional behavior. Off by default.
	Insecure bool

	// AllowUnsandboxed opts out of the fail-closed "refuse to run a job
	// when Docker is unavailable" default (T32). When false (default) and
	// Docker is not on PATH, runGateInDocker refuses the job instead of
	// silently executing the cloned repo's gate commands directly on the
	// host. When true, the previous runGateDirect fallback behavior
	// applies.
	AllowUnsandboxed bool

	// AllowedRepos is the set of "owner/repo" full names this server will
	// dispatch jobs for. Fail-closed by design: an EMPTY list rejects every
	// job (not "allow all") — an operator who configured zero repos almost
	// certainly forgot to configure this, not intentionally wants an open
	// relay that clones and executes whatever repository a validly-signed
	// (or, if --insecure, any) webhook payload names.
	AllowedRepos []string
}

// RunServe starts the webhook listener and blocks until SIGINT/SIGTERM.
func RunServe(cfg ServeConfig) error {
	// Resolve secret: flag > env.
	secret := cfg.Secret
	if secret == "" {
		secret = os.Getenv("GITHUB_WEBHOOK_SECRET")
	}
	if secret == "" {
		// T32: a webhook endpoint with no secret accepts and dispatches ANY
		// POST body with no authentication at all — a fully open trigger
		// for cloning and executing arbitrary repository content. Fail
		// closed by default; --insecure is the loud, explicit opt-out.
		if !cfg.Insecure {
			return fmt.Errorf("refusing to start: no webhook secret configured (--secret or GITHUB_WEBHOOK_SECRET); pass --insecure to start anyway (NOT recommended — this exposes an unauthenticated arbitrary-repo code-execution endpoint)")
		}
		ui.Warn("GITHUB_WEBHOOK_SECRET not set — webhook signature verification DISABLED (insecure)")
	}

	// Default concurrency.
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = "/tmp/nself-ci-workdirs"
	}
	if cfg.JobTimeout <= 0 {
		cfg.JobTimeout = 600
	}

	// Ensure workdir exists.
	if err := os.MkdirAll(cfg.WorkDir, 0o750); err != nil {
		return fmt.Errorf("cannot create workdir %s: %w", cfg.WorkDir, err)
	}

	// Build the nself-ci binary path once at startup.
	if cfg.ResolveGate == nil {
		return fmt.Errorf("nself-ci binary unavailable: no gate resolver configured")
	}
	binaryPath, err := cfg.ResolveGate(cfg.Verbose)
	if err != nil {
		return fmt.Errorf("nself-ci binary unavailable: %w", err)
	}

	// Worker pool: a buffered channel acts as a semaphore.
	sem := make(chan struct{}, cfg.Concurrency)

	handler := &webhookHandler{
		secret:     secret,
		sem:        sem,
		binaryPath: binaryPath,
		cfg:        cfg,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthz)
	// /webhook is the explicit GitHub webhook target URL.
	mux.HandleFunc("/webhook", handler.ServeHTTP)
	// / serves info on GET, webhooks on POST (supports GitHub configuring root path).
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handler.ServeHTTP(w, r)
			return
		}
		handleInfo(w, r, cfg)
	})

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", cfg.Addr, err)
	}

	ui.Section("nself-ci serve")
	ui.Info(fmt.Sprintf("listening on %s  concurrency=%d  timeout=%ds", cfg.Addr, cfg.Concurrency, cfg.JobTimeout))
	ui.Info(fmt.Sprintf("gate binary: %s", binaryPath))
	if secret == "" {
		ui.Warn("signature verification OFF — set GITHUB_WEBHOOK_SECRET for production")
	}

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case sig := <-quit:
		ui.Info(fmt.Sprintf("shutting down (%s) — draining jobs...", sig))
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}
}

// handleHealthz returns 200 OK for health probes.
func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "nself-ci-serve",
		"port":    "3845",
	})
}

// handleInfo returns a plaintext info page at /.
func handleInfo(w http.ResponseWriter, _ *http.Request, cfg ServeConfig) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "nself-ci-serve\n")
	_, _ = fmt.Fprintf(w, "  port        : 3845\n")
	_, _ = fmt.Fprintf(w, "  concurrency : %d\n", cfg.Concurrency)
	_, _ = fmt.Fprintf(w, "  job_timeout : %ds\n", cfg.JobTimeout)
	_, _ = fmt.Fprintf(w, "  workdir     : %s\n", cfg.WorkDir)
	_, _ = fmt.Fprintf(w, "  /healthz    : health probe\n")
	_, _ = fmt.Fprintf(w, "  POST /      : GitHub webhook (push, pull_request)\n")
}
