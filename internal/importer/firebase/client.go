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
	"time"

	"github.com/nself-org/cli/internal/httptimeout"
)

const maxPage = 64 << 20

type client struct {
	http    *http.Client
	tokens  tokenSource
	backoff func(attempt int) time.Duration
}

func newClient(t tokenSource) *client {
	return &client{http: httptimeout.NoProxy(5 * time.Minute), tokens: t, backoff: func(a int) time.Duration { return time.Duration(a) * 500 * time.Millisecond }}
}

type statusError struct{ Status int }

func (e statusError) Error() string { return fmt.Sprintf("source API returned HTTP %d", e.Status) }

// retryable statuses are retried a few times; everything else fails at once.
func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable || status == http.StatusBadGateway
}

// open sends the request and returns the response for a 2xx status.
func (c *client) open(ctx context.Context, method, url string, body any) (*http.Response, error) {
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
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, errors.New("source API unreachable")
		}
		if resp.StatusCode/100 == 2 {
			return resp, nil
		}
		_ = resp.Body.Close()
		if !retryable(resp.StatusCode) || attempt >= 4 {
			return nil, statusError{resp.StatusCode}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.backoff(attempt)):
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
