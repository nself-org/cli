package supabase

// Purpose: read buckets and objects through the Supabase storage REST API.
// Inputs: API base URL and service key. Outputs: bucket names, object keys and streamed bytes.
// Constraints: the key travels only in request headers to the configured API host; redirects
// are not followed; errors are fixed texts that carry neither URL nor key.
import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

const (
	listPage    = 100
	maxJSONBody = 16 << 20
)

type api struct {
	base *url.URL
	key  string
	hc   *http.Client
}

func apiErr() error { return errs.New("E520", "storage api request failed") }

// newAPI validates the API URL: https anywhere, plain http only to a loopback host (the local
// fixture), no credentials, query or path in the URL.
func newAPI(raw, key string) (*api, error) {
	u, err := url.Parse(raw)
	bad := errs.New("E517", "storage api url invalid")
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, bad
	}
	switch u.Scheme {
	case "https":
	case "http":
		ip := net.ParseIP(u.Hostname())
		if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, bad
		}
	default:
		return nil, bad
	}
	u.Path = ""
	hc := &http.Client{Timeout: 0, Transport: &http.Transport{ResponseHeaderTimeout: 60 * time.Second, TLSHandshakeTimeout: 15 * time.Second}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &api{base: u, key: key, hc: hc}, nil
}

func (a *api) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base.String()+path, rd)
	if err != nil {
		return nil, apiErr()
	}
	req.Header.Set("Authorization", "Bearer "+a.key)
	req.Header.Set("apikey", a.key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return nil, apiErr()
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, apiErr()
	}
	return resp, nil
}

func (a *api) json(ctx context.Context, method, path string, body []byte, out any) error {
	resp, err := a.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if json.NewDecoder(io.LimitReader(resp.Body, maxJSONBody)).Decode(out) != nil {
		return apiErr()
	}
	return nil
}

// buckets lists the bucket names, sorted by the server.
func (a *api) buckets(ctx context.Context) ([]string, error) {
	var rows []struct {
		ID string `json:"id"`
	}
	if err := a.json(ctx, http.MethodGet, "/storage/v1/bucket", nil, &rows); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.ID)
	}
	return names, nil
}

// keys returns every object key in a bucket, descending into folders. Folder entries have a
// null id in the listing.
func (a *api) keys(ctx context.Context, bucket string) ([]string, error) {
	var out []string
	var walk func(prefix string) error
	walk = func(prefix string) error {
		for offset := 0; ; offset += listPage {
			body, _ := json.Marshal(map[string]any{"prefix": prefix, "limit": listPage, "offset": offset, "sortBy": map[string]string{"column": "name", "order": "asc"}})
			var rows []struct {
				Name string  `json:"name"`
				ID   *string `json:"id"`
			}
			if err := a.json(ctx, http.MethodPost, "/storage/v1/object/list/"+url.PathEscape(bucket), body, &rows); err != nil {
				return err
			}
			for _, r := range rows {
				if r.ID == nil {
					if err := walk(prefix + r.Name + "/"); err != nil {
						return err
					}
					continue
				}
				out = append(out, prefix+r.Name)
			}
			if len(rows) < listPage {
				return nil
			}
		}
	}
	return out, walk("")
}

// get opens one object for streaming.
func (a *api) get(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	segs := strings.Split(key, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	resp, err := a.do(ctx, http.MethodGet, "/storage/v1/object/"+url.PathEscape(bucket)+"/"+strings.Join(segs, "/"), nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
