package firebase

// Purpose: read Cloud Storage objects through the GCS JSON API (the Firebase Storage emulator
// serves the same paths). Inputs: bucket name and base URL. Outputs: object names and streams.
// Constraints: the listing is read twice with different page sizes and the second total is the
// independent source count; object bytes stream straight into the bundle, never into memory.
import (
	"context"
	"errors"
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
	Name string `json:"name"`
	Size string `json:"size"`
}

// list returns every object name, reading pages of the given size.
func (g *gcs) list(ctx context.Context, maxResults int) ([]objectInfo, error) {
	var out []objectInfo
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
			return nil, err
		}
		out = append(out, page.Items...)
		if page.NextPageToken == "" {
			return out, nil
		}
		token = page.NextPageToken
	}
}

// open streams one object's bytes.
func (g *gcs) open(ctx context.Context, name string) (io.ReadCloser, error) {
	if name == "" || strings.IndexByte(name, 0) >= 0 {
		return nil, errors.New("object name invalid")
	}
	resp, err := g.c.open(ctx, "GET", g.base+"/storage/v1/b/"+url.PathEscape(g.bucket)+"/o/"+url.PathEscape(name)+"?alt=media", nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
