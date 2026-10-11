package scenarios

// Purpose: measure HTTP invocation overhead against the identical direct CLI.
// Inputs: a built binary and Config. Outputs: per-pair overhead samples in ms.
// Constraints: isolated fake project/HOME, bearer only in headers/environment,
// one server, bounded probes, and no production services (cli.perfbench v1).

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/nself-org/cli/internal/httptimeout"
)

const benchHTTPToken = "fake-token-0000"

type httpInvoke struct{}

func init()                     { Register(httpInvoke{}) }
func (httpInvoke) Name() string { return "http-invoke" }

// measureHTTPPairs alternates pair order to avoid a consistent warm-cache bias.
func measureHTTPPairs(ctx context.Context, cfg Config, httpRun, cliRun func(context.Context) (float64, []byte, error)) ([]Sample, error) {
	var samples []Sample
	for i := 0; i < cfg.Warmup+cfg.Runs; i++ {
		var hm, cm float64
		var hb, cb []byte
		var err error
		if i%2 == 0 {
			hm, hb, err = httpRun(ctx)
			if err == nil {
				cm, cb, err = cliRun(ctx)
			}
		} else {
			cm, cb, err = cliRun(ctx)
			if err == nil {
				hm, hb, err = httpRun(ctx)
			}
		}
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(hb, cb) {
			return nil, fmt.Errorf("http-invoke: HTTP and CLI envelopes differ")
		}
		if i >= cfg.Warmup {
			samples = append(samples, Sample{Metric: "http_invoke.overhead", Unit: "ms", Value: hm - cm})
		}
	}
	return samples, nil
}

func (httpInvoke) Run(ctx context.Context, cfg Config) ([]Sample, error) {
	bin, err := filepath.Abs(cfg.Bin)
	if err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp("", "perfbench-http-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	project, home := filepath.Join(scratch, "project"), filepath.Join(scratch, "home")
	for _, dir := range []string{project, home} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("PROJECT_NAME=surf\nENV=dev\nBASE_DOMAIN=local.nself.org\nPOSTGRES_PASSWORD=supersecretvalue123\n"), 0o600); err != nil {
		return nil, err
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		return nil, err
	}
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	env := append([]string{"PATH=" + os.Getenv("PATH"), "NSELF_TELEMETRY_OPT_OUT=1"}, cfg.Env...)
	env = append(env, "HOME="+home, "NSELF_MCP_TOKEN="+benchHTTPToken, "NSELF_V15=1")
	serverCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	server := exec.CommandContext(serverCtx, bin, "admin", "serve", "--port", strconv.Itoa(port))
	server.Dir, server.Env = project, env
	server.Stdout, server.Stderr = io.Discard, io.Discard
	// Let admin serve cancel and reap invocation children before forcing exit.
	server.Cancel = func() error { return server.Process.Signal(syscall.SIGTERM) }
	server.WaitDelay = 6 * time.Second
	if err := server.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- server.Wait() }()
	defer func() { cancel(); <-done }()
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	client := httptimeout.WithTimeout(timeout)
	if err := waitHTTPReady(ctx, client, base, timeout); err != nil {
		return nil, err
	}
	httpRun := func(ctx context.Context) (float64, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/commands/config/get", bytes.NewBufferString(`{"args":["BASE_DOMAIN"]}`))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+benchHTTPToken)
		req.Header.Set("Content-Type", "application/json")
		start := time.Now()
		res, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer res.Body.Close()
		b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		if res.StatusCode != http.StatusOK {
			return 0, nil, fmt.Errorf("http-invoke: status %d", res.StatusCode)
		}
		return float64(time.Since(start)) / float64(time.Millisecond), b, err
	}
	cliRun := func(ctx context.Context) (float64, []byte, error) {
		probeCtx, stop := context.WithTimeout(ctx, timeout)
		defer stop()
		cmd := exec.CommandContext(probeCtx, bin, "config", "get", "BASE_DOMAIN", "--json")
		cmd.Dir, cmd.Env, cmd.Stderr = project, env, io.Discard
		cmd.WaitDelay = time.Second
		start := time.Now()
		b, err := cmd.Output()
		return float64(time.Since(start)) / float64(time.Millisecond), b, err
	}
	return measureHTTPPairs(ctx, cfg, httpRun, cliRun)
}

func waitHTTPReady(ctx context.Context, client *http.Client, base string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/openapi.json", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+benchHTTPToken)
		res, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("http-invoke readiness: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
