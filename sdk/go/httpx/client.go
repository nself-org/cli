// Package httpx provides a standardized HTTP client with sensible defaults for
// nSelf plugins that call upstream APIs (OpenAI, Anthropic, etc.).
//
// SECURITY: this client has no SSRF guard. Use it only for fixed, trusted
// upstream URLs (OpenAI, Anthropic, ...). For user-supplied or dynamically
// built URLs, validate each URL with
// github.com/nself-org/plugin-sdk/httpx.ValidateOutboundURL before dialing (it
// rejects private, loopback and metadata addresses); this package does not call
// it.
package httpx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ClientOptions configures a retrying HTTP client.
type ClientOptions struct {
	Timeout    time.Duration // per-request timeout; default 30s
	UserAgent  string        // sent as User-Agent header; default "nself-plugin-sdk/0.1"
	MaxRetries int           // retry count for 5xx + network errors; default 2
	RetryDelay time.Duration // base delay between retries; default 500ms (exponential)
}

// Client wraps http.Client with retry + timeout defaults.
type Client struct {
	HTTP *http.Client
	opts ClientOptions
}

// New builds a Client with defaults applied.
func New(opts ClientOptions) *Client {
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "nself-plugin-sdk/0.1"
	}
	if opts.MaxRetries < 0 {
		opts.MaxRetries = 0
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = 2
	}
	if opts.RetryDelay == 0 {
		opts.RetryDelay = 500 * time.Millisecond
	}
	return &Client{
		HTTP: &http.Client{Timeout: opts.Timeout},
		opts: opts,
	}
}

// Do executes req with retry on 5xx + transport errors. Returns the final
// response regardless of status; caller decides how to handle 4xx.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}

	var (
		lastErr error
		resp    *http.Response
	)
	for attempt := 0; attempt <= c.opts.MaxRetries; attempt++ {
		if attempt > 0 {
			wait := c.opts.RetryDelay * (1 << (attempt - 1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		reqClone := req.Clone(ctx)
		resp, lastErr = c.HTTP.Do(reqClone)
		if lastErr != nil {
			continue
		}
		if resp.StatusCode < 500 {
			return resp, nil
		}
		// Drain body so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		lastErr = fmt.Errorf("httpx: upstream %d", resp.StatusCode)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return resp, nil
}
