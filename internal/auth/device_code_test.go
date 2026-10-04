// Package auth — contract tests for the device code login flow.
//
// The stub server answers with the recorded auth_server bodies in
// testdata/auth_server/ (copied from web routes/device-code.ts and
// session.ts). Nothing here contacts a real host: NSELF_AUTH_SERVER_URL always
// points at an httptest server and the wait between polls is faked.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// fixture returns the recorded auth_server body with the given file name.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "auth_server", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

// loginStub is an auth_server stand-in: it serves the device-code start route,
// replays a script of token-poll replies, and serves GET /auth/session.
type loginStub struct {
	t       *testing.T
	mu      sync.Mutex
	script  []string // fixture names for successive POST /auth/device-code/token calls
	session string   // fixture name served by GET /auth/session
	paths   []string // "METHOD path" of every request, in order
	bodies  map[string]string
	cookies []string
}

// newLoginStub starts the stub and points NSELF_AUTH_SERVER_URL at it.
func newLoginStub(t *testing.T, script ...string) *loginStub {
	t.Helper()
	s := &loginStub{t: t, script: script, session: "session_ok.json", bodies: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)
	return s
}

func (s *loginStub) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := r.Method + " " + r.URL.Path
	s.paths = append(s.paths, key)
	body, _ := io.ReadAll(r.Body)
	s.bodies[key] = string(body)
	w.Header().Set("Content-Type", "application/json")
	switch key {
	case "POST /auth/device-code":
		_, _ = w.Write(fixture(s.t, "device_code_start.json"))
	case "POST /auth/device-code/token":
		if len(s.script) == 0 {
			s.t.Errorf("unexpected extra token poll")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		name := s.script[0]
		s.script = s.script[1:]
		if name != "device_code_token_ok.json" {
			w.WriteHeader(http.StatusBadRequest) // auth_server answers every poll error with 400
		}
		_, _ = w.Write(fixture(s.t, name))
	case "GET /auth/session":
		s.cookies = append(s.cookies, r.Header.Get("Cookie"))
		if s.session == "session_unauthorized.json" {
			w.WriteHeader(http.StatusUnauthorized)
		}
		_, _ = w.Write(fixture(s.t, s.session))
	default:
		http.NotFound(w, r)
	}
}

// fakeSleep replaces the wait between polls with a recorder and returns the
// durations it was asked to wait.
func fakeSleep(t *testing.T) *[]time.Duration {
	t.Helper()
	var waits []time.Duration
	orig := sleepCtx
	sleepCtx = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		return ctx.Err()
	}
	t.Cleanup(func() { sleepCtx = orig })
	return &waits
}

func TestAuthServerURL_DefaultAndOverride(t *testing.T) {
	t.Setenv("NSELF_AUTH_SERVER_URL", "")
	if got := AuthServerURL(); got != "https://auth-server.nself.org" {
		t.Errorf("default: got %q, want https://auth-server.nself.org", got)
	}
	t.Setenv("NSELF_AUTH_SERVER_URL", "http://127.0.0.1:9999/")
	if got := AuthServerURL(); got != "http://127.0.0.1:9999" {
		t.Errorf("override (trailing slash trimmed): got %q", got)
	}
}

func TestDeviceAuthorize_Contract(t *testing.T) {
	stub := newLoginStub(t)
	got, err := DeviceAuthorize(context.Background())
	if err != nil {
		t.Fatalf("DeviceAuthorize: %v", err)
	}
	var want struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
		URI        string `json:"verification_uri"`
	}
	if err := json.Unmarshal(fixture(t, "device_code_start.json"), &want); err != nil {
		t.Fatal(err)
	}
	if got.DeviceCode != want.DeviceCode || got.UserCode != want.UserCode || got.VerificationURL != want.URI {
		t.Errorf("fields not decoded from verification_uri/user_code/device_code: %+v", got)
	}
	if got.ExpiresInSec != 300 || got.IntervalSec != 5 {
		t.Errorf("expires_in/interval: got %d/%d, want 300/5", got.ExpiresInSec, got.IntervalSec)
	}
	if len(stub.paths) != 1 || stub.paths[0] != "POST /auth/device-code" {
		t.Errorf("paths = %v, want only POST /auth/device-code", stub.paths)
	}
	if !strings.Contains(stub.bodies["POST /auth/device-code"], `"client_name"`) {
		t.Errorf("start body should name the client: %q", stub.bodies["POST /auth/device-code"])
	}
}

func TestDeviceAuthorize_MissingCodesIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"verification_uri":"https://nself.org/device"}`))
	}))
	defer srv.Close()
	t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)
	if _, err := DeviceAuthorize(context.Background()); err == nil {
		t.Fatal("a reply without device_code/user_code must be an error")
	}
}

func TestPollDeviceCode_PendingThenApproved(t *testing.T) {
	stub := newLoginStub(t, "device_code_token_authorization_pending.json",
		"device_code_token_authorization_pending.json", "device_code_token_ok.json")
	waits := fakeSleep(t)

	var polls int
	res, err := PollDeviceCode(context.Background(), "dc-1", func(time.Duration) { polls++ })
	if err != nil {
		t.Fatalf("PollDeviceCode: %v", err)
	}
	var ok struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(fixture(t, "device_code_token_ok.json"), &ok)
	if res.Token.AccessToken != ok.AccessToken {
		t.Errorf("AccessToken not stored from access_token")
	}
	if res.Token.Email != "owner@example.com" || res.Token.Tier != "plus" || res.Token.DisplayName != "Test Owner" {
		t.Errorf("identity not filled from /auth/session: %+v", res.Token)
	}
	if exp, perr := time.Parse(time.RFC3339, res.Token.ExpiresAt); perr != nil || time.Until(exp) < 89*24*time.Hour {
		t.Errorf("ExpiresAt %q should be about 90 days out (expires_in)", res.Token.ExpiresAt)
	}
	if polls != 3 || len(*waits) != 2 || (*waits)[0] != 5*time.Second || (*waits)[1] != 5*time.Second {
		t.Errorf("polls=%d waits=%v, want 3 polls and two 5s waits", polls, *waits)
	}
	if strings.Contains(strings.Join(stub.paths, ","), "/auth/device/") {
		t.Errorf("legacy device paths must not be used: %v", stub.paths)
	}
	if !strings.Contains(stub.bodies["POST /auth/device-code/token"], `"device_code":"dc-1"`) {
		t.Errorf("poll body: %q", stub.bodies["POST /auth/device-code/token"])
	}
	if len(stub.cookies) != 1 || stub.cookies[0] != "nself_auth_token="+ok.AccessToken {
		t.Errorf("session lookup must send the device token as the auth cookie")
	}
}

func TestPollDeviceCode_SlowDownAddsFiveSeconds(t *testing.T) {
	newLoginStub(t, "device_code_token_slow_down.json", "device_code_token_authorization_pending.json",
		"device_code_token_slow_down.json", "device_code_token_ok.json")
	waits := fakeSleep(t)
	if _, err := PollDeviceCode(context.Background(), "dc", nil); err != nil {
		t.Fatalf("PollDeviceCode: %v", err)
	}
	want := []time.Duration{10 * time.Second, 10 * time.Second, 15 * time.Second}
	if len(*waits) != len(want) {
		t.Fatalf("waits = %v, want %v", *waits, want)
	}
	for i := range want {
		if (*waits)[i] != want[i] {
			t.Errorf("wait %d = %v, want %v (slow_down is +5s, kept for the rest of the flow)", i, (*waits)[i], want[i])
		}
	}
}

func TestPollDeviceCode_StartsAtServerInterval(t *testing.T) {
	newLoginStub(t, "device_code_token_authorization_pending.json", "device_code_token_ok.json")
	waits := fakeSleep(t)
	if _, err := PollDeviceCode(context.Background(), "dc", nil, WithPollInterval(7*time.Second), WithPollInterval(0)); err != nil {
		t.Fatalf("PollDeviceCode: %v", err)
	}
	if len(*waits) != 1 || (*waits)[0] != 7*time.Second {
		t.Errorf("waits = %v, want [7s] (zero interval keeps the previous value)", *waits)
	}
}

func TestPollDeviceCode_TerminalErrors(t *testing.T) {
	cases := []struct {
		fixture string
		want    error
		hint    string
	}{
		{"device_code_token_expired_token.json", ErrDeviceCodeExpired, "nself login"},
		{"device_code_token_access_denied.json", ErrAccessDenied, "nself login"},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			newLoginStub(t, c.fixture)
			fakeSleep(t)
			res, err := PollDeviceCode(context.Background(), "secret-device-code", nil)
			if res != nil || !errors.Is(err, c.want) {
				t.Fatalf("got (%v, %v), want %v", res, err, c.want)
			}
			if !strings.Contains(err.Error(), c.hint) {
				t.Errorf("error should carry a remediation: %v", err)
			}
			if strings.Contains(err.Error(), "secret-device-code") {
				t.Errorf("error must not echo the device code: %v", err)
			}
		})
	}
}

func TestPollDeviceCode_UnknownServerErrorIsReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_request","message":"device_code required"}`))
	}))
	defer srv.Close()
	t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)
	fakeSleep(t)
	_, err := PollDeviceCode(context.Background(), "dc", nil)
	var ae *AuthAPIError
	if !errors.As(err, &ae) || ae.Code != "invalid_request" {
		t.Fatalf("want the server's invalid_request error, got %v", err)
	}
}

// A failed identity lookup after approval is an error: the login must not be
// reported with a blank email or a defaulted tier.
func TestPollDeviceCode_IdentityLookupFailureIsError(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"session 401", http.StatusUnauthorized, string(fixture(t, "session_unauthorized.json"))},
		{"session 500", http.StatusInternalServerError, `{"error":"internal_error","message":"boom"}`},
		{"tier missing", http.StatusOK, `{"authenticated":true,"account":{"email":"a@b.c","tier":""}}`},
		{"not authenticated", http.StatusOK, `{"authenticated":false}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/auth/session" {
					w.WriteHeader(c.status)
					_, _ = w.Write([]byte(c.body))
					return
				}
				_, _ = w.Write(fixture(t, "device_code_token_ok.json"))
			}))
			defer srv.Close()
			t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)
			fakeSleep(t)
			res, err := PollDeviceCode(context.Background(), "dc", nil)
			if res != nil || err == nil {
				t.Fatalf("want an error and no result, got (%v, %v)", res, err)
			}
			if strings.Contains(err.Error(), "eyJ") {
				t.Errorf("error must not contain the token: %v", err)
			}
		})
	}
}

func TestPollDeviceCode_Timeout(t *testing.T) {
	newLoginStub(t)
	orig := pollTimeout
	pollTimeout = -time.Second
	t.Cleanup(func() { pollTimeout = orig })
	if _, err := PollDeviceCode(context.Background(), "dc", nil); !errors.Is(err, ErrPollTimeout) {
		t.Fatalf("want ErrPollTimeout, got %v", err)
	}
}

func TestPollDeviceCode_ContextCancelledWhileWaiting(t *testing.T) {
	newLoginStub(t, "device_code_token_authorization_pending.json")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := PollDeviceCode(ctx, "dc", nil, WithPollInterval(time.Hour))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestPollToken_ApprovedWithoutAccessTokenIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token_type":"Bearer","expires_in":60}`))
	}))
	defer srv.Close()
	t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)
	if tok, err := PollToken(context.Background(), "dc"); err == nil || tok != nil {
		t.Fatalf("an approval with no access_token must be an error, got (%v, %v)", tok, err)
	}
}

// ─── E225: paths auth_server does not serve ───────────────────────────────────

// /account/devices and /account/team are served by no host today. A 404 (the
// Express default HTML page or a JSON body) is E225 with exit class 2, never a
// raw 404 and never an empty list.
func TestUnservedAccountPaths_E225(t *testing.T) {
	calls := map[string]func(context.Context, string) error{
		"/account/devices": func(ctx context.Context, tok string) error { _, err := GetDevices(ctx, tok); return err },
		"/account/team":    func(ctx context.Context, tok string) error { _, err := GetTeamMembers(ctx, tok); return err },
	}
	bodies := map[string]string{
		"express html": string(fixture(t, "not_found_express.html")),
		"json":         `{"error":"not_found","message":"no such route"}`,
	}
	for path, call := range calls {
		for bname, body := range bodies {
			t.Run(path+" "+bname, func(t *testing.T) {
				var gotPath string
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					gotPath = r.URL.Path
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(body))
				}))
				defer srv.Close()
				t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)

				err := call(context.Background(), "tok-secret")
				var ce *errs.CLIError
				if !errors.As(err, &ce) || ce.Code != "E225" {
					t.Fatalf("want an E225 CLIError, got %v", err)
				}
				if gotPath != path {
					t.Errorf("requested %q, want %q", gotPath, path)
				}
				msg := err.Error()
				if !strings.Contains(msg, "[E225]") || !strings.Contains(msg, "this server does not provide "+path) {
					t.Errorf("message: %q", msg)
				}
				if strings.Contains(msg, "tok-secret") || strings.Contains(msg, "Cannot GET") {
					t.Errorf("message leaks the token or the raw 404 body: %q", msg)
				}
				if got := errs.ExitCodeFor(err); got != 2 {
					t.Errorf("exit class = %d, want 2", got)
				}
			})
		}
	}
}

// Only a 404 maps to E225; other failures keep the server's own error, and
// /account/licenses (served by auth_server) is unchanged.
func TestUnservedAccountPaths_OtherStatusesUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"no licences"}`))
	}))
	defer srv.Close()
	t.Setenv("NSELF_AUTH_SERVER_URL", srv.URL)
	_, err := GetLicenses(context.Background(), "tok")
	var ae *AuthAPIError
	if !errors.As(err, &ae) || ae.Status != http.StatusNotFound {
		t.Fatalf("GetLicenses 404 should stay an AuthAPIError, got %v", err)
	}

	srv500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal_error","message":"boom"}`))
	}))
	defer srv500.Close()
	t.Setenv("NSELF_AUTH_SERVER_URL", srv500.URL)
	if _, err := GetDevices(context.Background(), "tok"); errors.As(err, new(*errs.CLIError)) {
		t.Errorf("a 500 must not become E225: %v", err)
	}
}
