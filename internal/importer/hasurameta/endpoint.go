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

// maxBody bounds one Hasura response; real metadata documents are far smaller.
const maxBody = 64 << 20

// Base validates raw and returns the endpoint root without a trailing slash or the
// /v1/graphql suffix users copy from a console. The admin secret is sent in clear text over
// http, so http is accepted for loopback hosts only; userinfo in the URL is refused.
func Base(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errs.New("E517", "source Hasura URL invalid")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !loopback(u.Hostname()) {
			return nil, errs.New("E517", "source Hasura URL must use https unless it is loopback")
		}
	default:
		return nil, errs.New("E517", "source Hasura URL invalid")
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
			return nil, errs.New("E517", "source Hasura request invalid")
		}
		rd = bytes.NewReader(b)
	}
	u := *base
	u.Path += path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return nil, errs.New("E517", "source Hasura request invalid")
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("X-Hasura-Admin-Secret", secret)
	}
	resp, err := httptimeout.Default.Do(req)
	if err != nil {
		return nil, errs.New("E517", "source Hasura unreachable")
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(b) > maxBody {
		return nil, errs.New("E517", "source Hasura response unreadable")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errs.New("E517", "source Hasura refused the admin secret")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errs.New("E517", "source Hasura request failed")
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
		return "", errs.New("E517", "source Hasura version unreadable")
	}
	return v.Version, nil
}

// Export returns the raw export_metadata document; a wrong admin secret fails here, so
// Detect (and --plan) catch it before any extraction work.
func Export(ctx context.Context, base *url.URL, adminSecret string) ([]byte, error) {
	return call(ctx, base, adminSecret, http.MethodPost, "/v1/metadata", map[string]any{"type": "export_metadata", "args": map[string]any{}})
}
