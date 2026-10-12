package firebase

// Purpose: one authenticated JSON/stream HTTP client shared by the Firestore and GCS readers.
// Inputs: a token source and base URLs. Outputs: decoded pages or response streams.
// Constraints: the bearer token only goes to the base URLs the settings chose; a redirect is never
// followed; errors carry the HTTP status only (a response body can echo request data).
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/nself-org/cli/internal/httptimeout"
)

const (
	maxPage     = 96 << 20
	maxAttempts = 7
)

type client struct {
	http *http.Client
	// bulk has no overall timeout, only a response-header timeout, so a large object can stream
	// for as long as it keeps flowing; the request context still bounds it.
	bulk    *http.Client
	tokens  tokenSource
	backoff func(attempt int) time.Duration
}

func newClient(t tokenSource) *client {
	bulk := httptimeout.NoProxy(0)
	if tr, ok := bulk.Transport.(*http.Transport); ok {
		tr.ResponseHeaderTimeout = 2 * time.Minute
		// Object bytes must arrive as stored: no transparent gzip handling on our side.
		tr.DisableCompression = true
	}
	return &client{http: httptimeout.NoProxy(5 * time.Minute), bulk: bulk, tokens: t, backoff: func(a int) time.Duration { return min(time.Duration(a*a)*500*time.Millisecond, 20*time.Second) }}
}

type statusError struct{ Status int }

func (e statusError) Error() string { return fmt.Sprintf("source API returned HTTP %d", e.Status) }

// retryable statuses are retried a few times; everything else fails at once.
func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusInternalServerError || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

// open sends the request and returns the response for a 2xx status.
func (c *client) open(ctx context.Context, method, url string, body any) (*http.Response, error) {
	return c.send(ctx, c.http, method, url, body)
}

// send is open with an explicit HTTP client.
func (c *client) send(ctx context.Context, hc *http.Client, method, url string, body any) (*http.Response, error) {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return nil, errors.New("request body invalid")
		}
	}
	for attempt := 1; ; attempt++ {
		token, err := c.tokens.Token(ctx)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
		if err != nil {
			return nil, errors.New("request invalid")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if hc == c.bulk {
			// Without this GCS inflates objects stored with Content-Encoding: gzip.
			req.Header.Set("Accept-Encoding", "gzip")
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		// Every request this importer sends only reads, so a retry is always safe.
		resp, err := hc.Do(req)
		wait := c.backoff(attempt)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt >= maxAttempts {
				return nil, errors.New("source API unreachable")
			}
		case resp.StatusCode/100 == 2:
			return resp, nil
		default:
			status, after := resp.StatusCode, resp.Header.Get("Retry-After")
			_ = resp.Body.Close()
			if !retryable(status) || attempt >= maxAttempts {
				return nil, statusError{status}
			}
			if secs, e := strconv.Atoi(after); e == nil && secs > 0 && time.Duration(secs)*time.Second > wait {
				wait = min(time.Duration(secs)*time.Second, time.Minute)
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// getJSON decodes one bounded JSON response into out.
func (c *client) getJSON(ctx context.Context, url string, out any) error {
	return c.doJSON(ctx, http.MethodGet, url, nil, out)
}

func (c *client) doJSON(ctx context.Context, method, url string, body, out any) error {
	resp, err := c.open(ctx, method, url, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxPage)).Decode(out); err != nil {
		return errors.New("source API response invalid")
	}
	return nil
}
