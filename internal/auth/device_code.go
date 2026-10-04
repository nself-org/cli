// Package auth — device code polling loop for the CLI login flow.
//
// Purpose: poll auth_server's POST /auth/device-code/token until the user
// approves the login in the browser, then look up who logged in.
// Contract source: web backend/services/auth_server/src/routes/device-code.ts
// (fixtures in testdata/auth_server/).
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	// defaultPollInterval is the polling interval between token requests
	// (auth_server enforces a 5 s minimum and answers slow_down below it).
	defaultPollInterval = 5 * time.Second
	// slowDownStep is added to the interval each time the server says slow_down.
	slowDownStep = 5 * time.Second
	// defaultPollTimeout is the maximum time to wait for user authorization.
	defaultPollTimeout = 5 * time.Minute
)

// Test seams: the poll window and the wait between polls.
var (
	pollTimeout = defaultPollTimeout
	sleepCtx    = sleepContext
)

// Device-code polling outcomes. PollToken maps the auth_server error codes
// authorization_pending (nil, nil), slow_down, expired_token and access_denied.
var (
	// ErrPollTimeout is returned when the device code polling window expires.
	ErrPollTimeout = fmt.Errorf("authorization timed out — the login URL has expired. Run 'nself login' to try again")
	// ErrDeviceCodeExpired means the server no longer knows the device code.
	ErrDeviceCodeExpired = errors.New("the login code has expired. Run 'nself login' to get a new code")
	// ErrAccessDenied means the user denied the login in the browser.
	ErrAccessDenied = errors.New("the login was denied in the browser. Run 'nself login' to try again")
	// ErrSlowDown means the server asked the client to poll less often.
	ErrSlowDown = errors.New("auth server asked the CLI to poll more slowly")
)

// PollResult holds the outcome of a completed device code poll.
type PollResult struct {
	Token *TokenResponse
}

// PollOption adjusts PollDeviceCode.
type PollOption func(*pollConfig)

type pollConfig struct{ interval time.Duration }

// WithPollInterval sets the starting poll interval (the `interval` the server
// returned from the start call). Values of zero or less keep the default.
func WithPollInterval(d time.Duration) PollOption {
	return func(c *pollConfig) {
		if d > 0 {
			c.interval = d
		}
	}
}

// PollToken makes one POST /auth/device-code/token call.
//
// Returns (token, nil) when the user approved; the token carries only the
// access token and its expiry, identity comes from PollDeviceCode.
// Returns (nil, nil) for authorization_pending (and a bare 202).
// Returns ErrSlowDown, ErrDeviceCodeExpired or ErrAccessDenied for the matching
// server error codes, and the server error for anything else.
func PollToken(ctx context.Context, deviceCode string) (*TokenResponse, error) {
	url := fmt.Sprintf("%s/auth/device-code/token", AuthServerURL())

	body, _ := json.Marshal(map[string]string{"device_code": deviceCode})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("polling token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusAccepted {
		return nil, nil
	}

	if resp.StatusCode == http.StatusOK {
		var reply struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
			return nil, fmt.Errorf("parsing token response: %w", err)
		}
		if reply.AccessToken == "" {
			return nil, errors.New("auth server approved the login but sent no access_token")
		}
		tok := &TokenResponse{AccessToken: reply.AccessToken}
		if reply.ExpiresIn > 0 {
			tok.ExpiresAt = time.Now().UTC().Add(time.Duration(reply.ExpiresIn) * time.Second).Format(time.RFC3339)
		}
		return tok, nil
	}

	apiErr := parseAPIError(resp)
	var ae *AuthAPIError
	if errors.As(apiErr, &ae) {
		switch ae.Code {
		case "authorization_pending":
			return nil, nil
		case "slow_down":
			return nil, ErrSlowDown
		case "expired_token":
			return nil, ErrDeviceCodeExpired
		case "access_denied":
			return nil, ErrAccessDenied
		}
	}
	return nil, apiErr
}

// PollDeviceCode polls the auth server until the user authorizes the device code,
// the timeout expires, or the context is cancelled. On approval it looks up the
// account (GET /auth/session) and fills Email, Tier and DisplayName.
//
// onPoll is called before each poll attempt (use for progress indicators).
// Pass nil to skip the callback.
//
// A slow_down reply raises the interval by 5 s for the rest of the flow.
// Returns ErrPollTimeout if the poll window expires without user action, the
// ErrDeviceCodeExpired / ErrAccessDenied outcomes (wrapped), and an error when
// the account lookup after approval fails: a login is never reported as
// successful with a blank identity or a defaulted tier.
// Returns context.Canceled/context.DeadlineExceeded if ctx is cancelled.
func PollDeviceCode(ctx context.Context, deviceCode string, onPoll func(elapsed time.Duration), opts ...PollOption) (*PollResult, error) {
	cfg := pollConfig{interval: defaultPollInterval}
	for _, o := range opts {
		o(&cfg)
	}
	start := time.Now()
	deadline := start.Add(pollTimeout)

	for {
		// Check context cancellation first.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return nil, ErrPollTimeout
		}

		if onPoll != nil {
			onPoll(time.Since(start))
		}

		token, err := PollToken(ctx, deviceCode)
		switch {
		case errors.Is(err, ErrSlowDown):
			cfg.interval += slowDownStep
		case err != nil:
			return nil, fmt.Errorf("polling for authorization: %w", err)
		case token != nil:
			if err := fillIdentity(ctx, token); err != nil {
				return nil, err
			}
			return &PollResult{Token: token}, nil
		}

		// Wait for the next poll interval, respecting context cancellation.
		if err := sleepCtx(ctx, cfg.interval); err != nil {
			return nil, err
		}
	}
}

// fillIdentity completes tok with the account the device token belongs to.
func fillIdentity(ctx context.Context, tok *TokenResponse) error {
	info, err := GetSession(ctx, tok.AccessToken)
	if err != nil {
		return fmt.Errorf("login was approved but looking up the account failed: %w", err)
	}
	if !info.Authenticated || info.Account.Email == "" || info.Account.Tier == "" {
		return errors.New("login was approved but the auth server returned no account details; run 'nself login' again")
	}
	tok.Email = info.Account.Email
	tok.Tier = info.Account.Tier
	tok.DisplayName = info.Account.DisplayName
	return nil
}

// sleepContext waits d or until ctx is done.
func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
