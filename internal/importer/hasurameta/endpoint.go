package hasurameta

// Purpose: validate a Hasura endpoint, then export its metadata over HTTP.
// Inputs: base URL and admin secret. Outputs: raw export_metadata JSON.
// Constraints: no secret, URL or response body in errors; responses are size-bounded.
import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/httptimeout"
)

// Proxies: the client keeps the environment's proxy settings so Hasura Cloud stays reachable from a
// corporate network. That is safe for the admin secret: Go never proxies loopback, https goes
// through a CONNECT tunnel the proxy cannot read, and plain http to any other host is refused by Base.
//
// client never follows a redirect: Go would re-send the custom admin-secret header to the new
// location, so a redirecting endpoint (or a man in the middle) could collect the secret.
var client = func() *http.Client {
	c := httptimeout.WithTimeout(httptimeout.Default.Timeout)
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}()

// maxBody bounds one Hasura response; real metadata documents are far smaller.
const maxBody = 64 << 20

// Base validates raw and returns the endpoint root without a trailing slash or the
// /v1/graphql suffix users copy from a console. The admin secret is sent in clear text over
// http, so http is accepted for loopback hosts only; userinfo in the URL is refused.
func Base(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errs.New("E517", msgURLInvalid)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !loopback(u.Hostname()) {
			return nil, errs.New("E517", msgURLHTTP)
		}
	default:
		return nil, errs.New("E517", msgURLInvalid)
	}
	u.Path = strings.TrimSuffix(strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1/graphql"), "/")
	return u, nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func call(ctx context.Context, base *url.URL, secret, method, path string, body any) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, errs.New("E517", msgRequestInvalid)
		}
		rd = bytes.NewReader(b)
	}
	u := *base
	u.Path += path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return nil, errs.New("E517", msgRequestInvalid)
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("X-Hasura-Admin-Secret", secret)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errs.New("E517", msgUnreachable)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(b) > maxBody {
		return nil, errs.New("E517", msgResponse)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errs.New("E517", msgRefusedSecret)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errs.New("E517", msgRequestFailed)
	}
	return b, nil
}

// Version returns the Hasura server version from the unauthenticated /v1/version route.
func Version(ctx context.Context, base *url.URL) (string, error) {
	b, err := call(ctx, base, "", http.MethodGet, "/v1/version", nil)
	if err != nil {
		return "", err
	}
	var v struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(b, &v) != nil || v.Version == "" {
		return "", errs.New("E517", msgVersion)
	}
	return v.Version, nil
}

// Export returns the raw export_metadata document; a wrong admin secret fails here, so
// Detect (and --plan) catch it before any extraction work.
func Export(ctx context.Context, base *url.URL, adminSecret string) ([]byte, error) {
	return call(ctx, base, adminSecret, http.MethodPost, "/v1/metadata", map[string]any{"type": "export_metadata", "args": map[string]any{}})
}
