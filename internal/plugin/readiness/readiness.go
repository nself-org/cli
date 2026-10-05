// Package readiness verifies that a plugin has applied its own boot-time
// migrations (contract:plugin.boot-migrations v1, ADR 0010).
//
// Purpose: the CLI never applies plugin SQL. A plugin applies its migrations
// when it boots (sdk/go/migrate) and serves {"migrations":{"applied":n,
// "expected":m}} in its /health JSON. Install and update flows call Wait to
// block, bounded, until applied == expected; `nself doctor` calls Probe once.
//
// Inputs: the plugin name (for messages), its /health URL and a timeout.
// Outputs: nil when ready, otherwise a *errs.CLIError naming the plugin:
// E118 when the wait ended with applied < expected (or the plugin never
// answered), E119 when /health has no valid migrations field. A cancelled
// caller context returns the context error, not E118.
// Constraints: every wait is bounded (a non-positive timeout is an error);
// HTTP goes through internal/httptimeout; nothing here touches the database.
package readiness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/httptimeout"
)

const (
	// DefaultTimeout bounds Wait when NSELF_PLUGIN_READY_TIMEOUT is unset.
	DefaultTimeout = 120 * time.Second
	// EnvTimeout overrides DefaultTimeout (integer seconds, or a Go duration such as 3m).
	EnvTimeout = "NSELF_PLUGIN_READY_TIMEOUT"

	pollInterval = 2 * time.Second
	probeTimeout = 5 * time.Second
	maxBody      = 1 << 20
)

// Progress is the migrations status a plugin serves in /health.
type Progress struct {
	Applied  int
	Expected int
}

// Ready reports whether every expected migration is applied.
func (p Progress) Ready() bool { return p.Applied == p.Expected }

// unreachableError marks a probe that got no usable /health answer yet (the
// container is still booting). Wait retries it; Probe returns it.
type unreachableError struct{ cause error }

func (e *unreachableError) Error() string { return e.cause.Error() }
func (e *unreachableError) Unwrap() error { return e.cause }

// IsUnreachable reports whether err is a "plugin did not answer /health" error
// from Probe (as opposed to an E119 contract violation).
func IsUnreachable(err error) bool {
	var u *unreachableError
	return errors.As(err, &u)
}

// TimeoutFromEnv returns the wait bound: EnvTimeout when set, else DefaultTimeout.
// An unparseable or non-positive value is an error, never a silent default.
func TimeoutFromEnv() (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(EnvTimeout))
	if v == "" {
		return DefaultTimeout, nil
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n <= 0 {
			return 0, fmt.Errorf("%s=%q: must be a positive number of seconds", EnvTimeout, v)
		}
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s=%q: want positive seconds (120) or a duration (2m)", EnvTimeout, v)
	}
	return d, nil
}

// Option tunes Wait.
type Option func(*config)

type config struct {
	interval time.Duration
	client   *http.Client
}

// WithInterval sets the poll interval (default 2 s).
func WithInterval(d time.Duration) Option { return func(c *config) { c.interval = d } }

// WithClient sets the HTTP client (default httptimeout.WithTimeout(5 s)).
func WithClient(h *http.Client) Option { return func(c *config) { c.client = h } }

func newConfig(opts []Option) config {
	c := config{interval: pollInterval}
	for _, o := range opts {
		o(&c)
	}
	if c.interval <= 0 {
		c.interval = pollInterval
	}
	if c.client == nil {
		c.client = httptimeout.WithTimeout(probeTimeout)
	}
	return c
}

// Probe reads a plugin's /health once and returns its migrations progress.
// A plugin that answers 200 without a valid migrations field gets E119; a
// plugin that does not answer (connection error, non-200) gets an error for
// which IsUnreachable is true.
func Probe(ctx context.Context, client *http.Client, plugin, healthURL string) (Progress, error) {
	if client == nil {
		client = httptimeout.WithTimeout(probeTimeout)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return Progress{}, fmt.Errorf("plugin %s: bad health URL %q: %w", plugin, healthURL, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Progress{}, &unreachableError{fmt.Errorf("plugin %s: GET %s: %w", plugin, healthURL, err)}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Progress{}, &unreachableError{fmt.Errorf("plugin %s: reading %s: %w", plugin, healthURL, err)}
	}
	if resp.StatusCode != http.StatusOK {
		return Progress{}, &unreachableError{fmt.Errorf("plugin %s: %s returned HTTP %d", plugin, healthURL, resp.StatusCode)}
	}
	return decode(plugin, body)
}

func decode(plugin string, body []byte) (Progress, error) {
	bad := func(why string) error {
		return errs.Newf("E119", "plugin %s: /health has no valid migrations status (%s)", plugin, why)
	}
	var doc struct {
		Migrations *struct {
			Applied  *int `json:"applied"`
			Expected *int `json:"expected"`
		} `json:"migrations"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return Progress{}, bad("body is not a JSON object")
	}
	m := doc.Migrations
	switch {
	case m == nil:
		return Progress{}, bad("field absent")
	case m.Applied == nil || m.Expected == nil:
		return Progress{}, bad("applied and expected are both required")
	case *m.Applied < 0 || *m.Expected < 0:
		return Progress{}, bad("negative count")
	}
	return Progress{Applied: *m.Applied, Expected: *m.Expected}, nil
}

// Wait polls healthURL until the plugin reports applied == expected, for at
// most timeout. It returns E119 at once when /health lacks the migrations
// field, and E118 (naming the plugin and both numbers) when the timeout ends
// first. Callers read the bound with TimeoutFromEnv.
func Wait(ctx context.Context, plugin, healthURL string, timeout time.Duration, opts ...Option) error {
	if timeout <= 0 {
		return fmt.Errorf("plugin %s: readiness wait needs a positive timeout, got %s", plugin, timeout)
	}
	cfg := newConfig(opts)
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var last Progress
	var lastErr error
	seen := false
	for {
		p, err := Probe(wctx, cfg.client, plugin, healthURL)
		switch {
		case err == nil && p.Ready():
			return nil
		case err == nil:
			last, seen, lastErr = p, true, nil
		case IsUnreachable(err):
			lastErr = err
		default:
			return err // E119 or a bad URL: waiting longer cannot fix it
		}
		select {
		case <-wctx.Done():
			if cerr := ctx.Err(); cerr != nil {
				return fmt.Errorf("plugin %s: waiting for migrations: %w", plugin, cerr)
			}
			return timeoutError(plugin, timeout, last, seen, lastErr)
		case <-time.After(cfg.interval):
		}
	}
}

func timeoutError(plugin string, timeout time.Duration, last Progress, seen bool, lastErr error) error {
	if seen {
		return errs.Newf("E118", "plugin %s: migrations applied %d of %d after %s", plugin, last.Applied, last.Expected, timeout)
	}
	e := errs.Newf("E118", "plugin %s: no migrations status after %s (health never answered)", plugin, timeout)
	if lastErr != nil {
		e.Wrapped = lastErr
	}
	return e
}
