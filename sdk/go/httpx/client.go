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

// maxBufferedBody caps how much of a body without GetBody Do copies into
// memory so it can retry. A larger body is sent once, without retries. A var
// so tests can lower it.
var maxBufferedBody int64 = 8 << 20

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
// on each attempt. A body over 8 MiB without GetBody is sent once, without
// retries; set GetBody to retry large bodies. Buffered bodies must match an
// explicit positive ContentLength. Inferred lengths apply only to attempts,
// preserving the caller's ContentLength and chunked encoding for trailers.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}

	// Keep buffering's metadata changes local. Share Trailer until the body
	// has been read, since a reader may populate its values at EOF.
	bufferReq := *req
	req = &bufferReq
	getBody, buffered, err := bodySource(req, c.opts.MaxRetries > 0)
	if err != nil {
		return nil, err
	}
	maxRetries := c.opts.MaxRetries
	if req.Body != nil && req.Body != http.NoBody && getBody == nil {
		// Nothing to replay: a single attempt, or a body too large to buffer.
		maxRetries = 0
	}

	var (
		lastErr error
		resp    *http.Response
	)
	for attempt := 0; attempt <= maxRetries; attempt++ {
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
				body, berr := getBody()
				if berr != nil {
					return nil, fmt.Errorf("httpx: rewind request body: %w", berr)
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
// each attempt. It returns nil when there is nothing to replay: no body, a
// single attempt (retries false), or a body without GetBody larger than
// maxBufferedBody (req.Body is then rebuilt so the one attempt still sends it
// whole). buffered reports that req.Body was read into memory and closed, so
// even the first attempt must use the copy. req is Do's local request copy:
// its ContentLength is inferred only when there are no trailers, after
// validating any explicit positive length against the buffered bytes.
func bodySource(req *http.Request, retries bool) (getBody func() (io.ReadCloser, error), buffered bool, err error) {
	if !retries || req.Body == nil || req.Body == http.NoBody {
		return nil, false, nil
	}
	if req.GetBody != nil {
		return req.GetBody, false, nil
	}
	orig := req.Body
	data, err := io.ReadAll(io.LimitReader(orig, maxBufferedBody+1))
	if err != nil {
		_ = orig.Close()
		return nil, false, fmt.Errorf("httpx: read request body: %w", err)
	}
	if int64(len(data)) > maxBufferedBody {
		req.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(data), orig), orig}
		return nil, false, nil
	}
	_ = orig.Close()
	length := int64(len(data))
	if req.ContentLength > 0 && req.ContentLength != length {
		return nil, false, fmt.Errorf("httpx: ContentLength=%d with buffered body length %d", req.ContentLength, length)
	}
	if len(req.Trailer) == 0 {
		req.ContentLength = length
	}
	return func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}, true, nil
}
