package firebase

// Purpose: read Cloud Storage objects through the GCS JSON API (the Firebase Storage emulator
// serves the same paths). Inputs: bucket name and base URL. Outputs: object names and streams.
// Constraints: the listing is read twice with different page sizes and the second total is the
// independent source count; object bytes stream straight into the bundle, never into memory.
import (
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
)

type gcs struct {
	c      *client
	base   string
	bucket string
}

type objectInfo struct {
	Name    string `json:"name"`
	Size    string `json:"size"`
	MD5Hash string `json:"md5Hash"`
}

// pages reads the bucket listing in pages of the given size and passes each item to fn.
func (g *gcs) pages(ctx context.Context, maxResults int, fn func(objectInfo)) error {
	token := ""
	for {
		u := g.base + "/storage/v1/b/" + url.PathEscape(g.bucket) + "/o?maxResults=" + strconv.Itoa(maxResults)
		if token != "" {
			u += "&pageToken=" + url.QueryEscape(token)
		}
		var page struct {
			Items         []objectInfo `json:"items"`
			NextPageToken string       `json:"nextPageToken"`
		}
		if err := g.c.getJSON(ctx, u, &page); err != nil {
			return err
		}
		for _, it := range page.Items {
			fn(it)
		}
		if page.NextPageToken == "" {
			return nil
		}
		token = page.NextPageToken
	}
}

// list returns every object name.
func (g *gcs) list(ctx context.Context, maxResults int) ([]objectInfo, error) {
	var out []objectInfo
	err := g.pages(ctx, maxResults, func(it objectInfo) { out = append(out, it) })
	return out, err
}

// count returns the listing total without keeping the items.
func (g *gcs) count(ctx context.Context, maxResults int) (int64, error) {
	var n int64
	err := g.pages(ctx, maxResults, func(objectInfo) { n++ })
	return n, err
}

// open streams one object's bytes.
func (g *gcs) open(ctx context.Context, name string) (io.ReadCloser, error) {
	if name == "" || strings.IndexByte(name, 0) >= 0 {
		return nil, safe("object name invalid")
	}
	resp, err := g.c.send(ctx, g.c.bulk, "GET", g.base+"/storage/v1/b/"+url.PathEscape(g.bucket)+"/o/"+url.PathEscape(name)+"?alt=media", nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
