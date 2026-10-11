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
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultMaxRetries is the retry count callers set explicitly to get the
// pre-D-0152 behaviour of a zero ClientOptions (two retries). MaxRetries 0
// now means no retries.
const DefaultMaxRetries = 2

// ClientOptions configures a retrying HTTP client.
type ClientOptions struct {
	Timeout    time.Duration // per-request timeout; default 30s
	UserAgent  string        // sent as User-Agent header; default "nself-plugin-sdk/0.1"
	MaxRetries int           // retries after the first attempt on 5xx + network errors; 0 (or negative) = no retries
	RetryDelay time.Duration // base delay between retries; default 500ms (exponential)
}

// Client wraps http.Client with retry + timeout defaults.
type Client struct {
	HTTP *http.Client
	opts ClientOptions
}

// New builds a Client with defaults applied. MaxRetries is honoured as given:
// 0 sends each request once, a negative value is treated as 0.
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
	if opts.RetryDelay == 0 {
		opts.RetryDelay = 500 * time.Millisecond
	}
	return &Client{
		HTTP: &http.Client{Timeout: opts.Timeout},
		opts: opts,
	}
}

// Do executes req with retry on 5xx + transport errors. Any response below
// 500 is returned as is; the caller decides how to handle 4xx. When every
// attempt fails, Do returns the last error and a nil response.
//
// Every attempt sends the full request body. If retries are enabled and the
// request has a body but no GetBody (a body built from a plain io.Reader),
// Do reads the body into memory once and closes it, then replays those bytes
// on each attempt.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}

	getBody, buffered, err := bodySource(req, c.opts.MaxRetries > 0)
	if err != nil {
		return nil, err
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
		if getBody != nil {
			reqClone.GetBody = getBody
			// The first attempt can send the caller's own body unless Do
			// already consumed it into the buffer.
			if attempt > 0 || buffered {
				body, err := getBody()
				if err != nil {
					return nil, fmt.Errorf("httpx: rewind request body: %w", err)
				}
				reqClone.Body = body
			}
		}
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

// bodySource returns a function that yields a fresh copy of req's body for
// each attempt. It returns nil when there is nothing to replay: no body, or
// a single attempt (retries false). buffered reports that req.Body was read
// into memory and closed, so even the first attempt must use the copy.
func bodySource(req *http.Request, retries bool) (getBody func() (io.ReadCloser, error), buffered bool, err error) {
	if !retries || req.Body == nil || req.Body == http.NoBody {
		return nil, false, nil
	}
	if req.GetBody != nil {
		return req.GetBody, false, nil
	}
	data, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, false, fmt.Errorf("httpx: read request body: %w", err)
	}
	return func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}, true, nil
}
