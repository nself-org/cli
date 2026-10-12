// Package appwrite imports an Appwrite project through its REST API.
// Purpose: source adapter for `nself db import appwrite`. Inputs: endpoint, project id and an
// API key read from a named environment variable. Outputs: a portable bundle (db, auth, storage,
// hasura). Constraints: the key never appears in argv, errors, logs or the report; only fixed
// credential-free messages leave this package.
package appwrite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/httptimeout"
)

const (
	pageSize     = 100
	maxPageBytes = 64 << 20
)

// client is a minimal Appwrite REST client. The key is held only in the request header.
type client struct {
	base, project, key string
	http, dl           *http.Client
}

func newClient(endpoint, project, key string) (*client, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errs.New("E517", "source connection configuration invalid")
	}
	if u.Scheme == "http" && !loopback(u.Hostname()) {
		return nil, errs.New("E517", "source connection configuration invalid")
	}
	if !idRe.MatchString(project) {
		return nil, errs.New("E517", "source connection configuration invalid")
	}
	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, "/v1") {
		path += "/v1"
	}
	u.Path = path
	return &client{base: u.String(), project: project, key: key, http: httptimeout.NoProxy(60 * time.Second), dl: httptimeout.NoProxy(2 * time.Hour)}, nil
}

// loopback reports whether host is localhost or a loopback address: the only place the API key
// may travel over plain http.
func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *client) do(ctx context.Context, hc *http.Client, path string, q url.Values) (*http.Response, error) {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errs.New("E517", "source connection configuration invalid")
	}
	req.Header.Set("X-Appwrite-Project", c.project)
	req.Header.Set("X-Appwrite-Key", c.key)
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, errs.New("E517", "source database unreachable")
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, errs.New("E517", "source connection configuration invalid")
		}
		return nil, errs.New("E517", "source schema lookup failed")
	}
	return resp, nil
}

func (c *client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	resp, err := c.do(ctx, c.http, path, q)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if json.NewDecoder(io.LimitReader(resp.Body, maxPageBytes)).Decode(out) != nil {
		return errs.New("E517", "source schema lookup failed")
	}
	return nil
}

// queryJSON renders one Appwrite query object (the 1.4+ JSON form).
func queryJSON(method string, values ...any) string {
	b, _ := json.Marshal(map[string]any{"method": method, "values": values})
	return string(b)
}

// page is a list response: the server-side total plus the raw items under key.
type page struct {
	Total int64
	Items []json.RawMessage
}

// listPage fetches one page of the collection under key. A non-empty after is a $id cursor;
// a positive offset pages by position instead (attribute documents carry no $id).
func (c *client) listPage(ctx context.Context, path, key, after string, offset int) (page, error) {
	q := url.Values{}
	q.Add("queries[]", queryJSON("limit", pageSize))
	if after != "" {
		q.Add("queries[]", queryJSON("cursorAfter", after))
	}
	if offset > 0 {
		q.Add("queries[]", queryJSON("offset", offset))
	}
	var raw map[string]json.RawMessage
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return page{}, err
	}
	var p page
	if t, ok := raw["total"]; ok && json.Unmarshal(t, &p.Total) != nil {
		return page{}, errs.New("E517", "source schema lookup failed")
	}
	if items, ok := raw[key]; ok && json.Unmarshal(items, &p.Items) != nil {
		return page{}, errs.New("E517", "source schema lookup failed")
	}
	return p, nil
}

// each walks every page of a list, calling fn per item, and returns the first page's total
// (Review F15: the independent source count comes from `total`, not from the rows read).
func (c *client) each(ctx context.Context, path, key string, fn func(json.RawMessage) error) (int64, error) {
	return c.walk(ctx, path, key, true, fn)
}

// walk is each with a choice of cursor ($id) or offset paging.
func (c *client) walk(ctx context.Context, path, key string, cursor bool, fn func(json.RawMessage) error) (int64, error) {
	after, total, seen := "", int64(0), 0
	for first := true; ; first = false {
		offset := 0
		if !cursor {
			offset = seen
		}
		p, err := c.listPage(ctx, path, key, after, offset)
		if err != nil {
			return 0, err
		}
		if first {
			total = p.Total
		}
		for _, item := range p.Items {
			if cursor {
				var id struct {
					ID string `json:"$id"`
				}
				if json.Unmarshal(item, &id) != nil || id.ID == "" {
					return 0, errs.New("E517", "source schema lookup failed")
				}
				after = id.ID
			}
			if err = fn(item); err != nil {
				return 0, err
			}
			seen++
		}
		if len(p.Items) < pageSize {
			return total, nil
		}
	}
}

func (c *client) download(ctx context.Context, bucket, file string) (io.ReadCloser, error) {
	resp, err := c.do(ctx, c.dl, fmt.Sprintf("/storage/buckets/%s/files/%s/download", url.PathEscape(bucket), url.PathEscape(file)), nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
