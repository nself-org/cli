package objstore

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// maxListBody bounds one ListObjectsV2 response.
const maxListBody = 64 << 20

// Object is one listed object.
type Object struct {
	Key  string
	Size int64
	ETag string
}

// Page is one ListObjectsV2 page.
type Page struct {
	Objects []Object
	// NextToken is the continuation token of the next page; empty when done.
	NextToken string
}

// Limits of List when the Client sets none.
const (
	DefaultMaxListPages   = 100_000
	DefaultMaxListObjects = 10_000_000
)

type listResult struct {
	IsTruncated  bool   `xml:"IsTruncated"`
	NextToken    string `xml:"NextContinuationToken"`
	EncodingType string `xml:"EncodingType"`
	Contents     []struct {
		Key  string `xml:"Key"`
		Size int64  `xml:"Size"`
		ETag string `xml:"ETag"`
	} `xml:"Contents"`
}

// ListPage returns one ListObjectsV2 page for prefix, starting at token ("" for
// the first page). The continuation token is URL-encoded on the wire. The
// request asks for encoding-type=url, so a key with a character XML cannot
// carry (U+0001 and the like) still lists; keys are decoded here when the
// server says it encoded them.
func (c *Client) ListPage(ctx context.Context, bucket, prefix, token string) (Page, error) {
	if err := checkBucket(bucket); err != nil {
		return Page{}, err
	}
	q := [][2]string{{"list-type", "2"}, {"encoding-type", "url"}}
	if prefix != "" {
		q = append(q, [2]string{"prefix", prefix})
	}
	if token != "" {
		q = append(q, [2]string{"continuation-token", token})
	}
	if c.PageSize > 0 {
		q = append(q, [2]string{"max-keys", strconv.Itoa(c.PageSize)})
	}
	req, err := c.build(ctx, http.MethodGet, bucket, "", q, emptySHA256)
	if err != nil {
		return Page{}, err
	}
	resp, err := c.send(ctx, "list", req)
	if err != nil {
		return Page{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Page{}, c.apiError("list", resp)
	}
	defer func() { _ = resp.Body.Close() }()
	var lr listResult
	if err := xml.NewDecoder(io.LimitReader(resp.Body, maxListBody)).Decode(&lr); err != nil {
		return Page{}, fmt.Errorf("objstore: list: cannot parse response: %w", err)
	}
	p := Page{Objects: make([]Object, 0, len(lr.Contents))}
	for _, o := range lr.Contents {
		key := o.Key
		if strings.EqualFold(lr.EncodingType, "url") {
			dec, err := url.QueryUnescape(key)
			if err != nil {
				return Page{}, errors.New("objstore: list: a key in the response is not validly URL-encoded")
			}
			key = dec
		}
		p.Objects = append(p.Objects, Object{Key: key, Size: o.Size, ETag: o.ETag})
	}
	if lr.IsTruncated {
		if lr.NextToken == "" {
			return Page{}, errors.New("objstore: list: truncated response without a continuation token")
		}
		p.NextToken = lr.NextToken
	}
	return p, nil
}

// List returns every object under prefix, sorted by key, following
// continuation tokens. A repeated token (a server that never advances) is an
// error, not an endless loop, and so is a listing past MaxListPages pages or
// MaxListObjects objects (a hostile endpoint cannot grow memory without bound).
func (c *Client) List(ctx context.Context, bucket, prefix string) ([]Object, error) {
	maxPages, maxObjects := c.MaxListPages, c.MaxListObjects
	if maxPages <= 0 {
		maxPages = DefaultMaxListPages
	}
	if maxObjects <= 0 {
		maxObjects = DefaultMaxListObjects
	}
	var all []Object
	seen := map[string]bool{}
	token := ""
	for pages := 1; ; pages++ {
		p, err := c.ListPage(ctx, bucket, prefix, token)
		if err != nil {
			return nil, err
		}
		if len(all)+len(p.Objects) > maxObjects {
			return nil, fmt.Errorf("objstore: list: more than %d objects under %q", maxObjects, prefix)
		}
		all = append(all, p.Objects...)
		if p.NextToken == "" {
			break
		}
		if seen[p.NextToken] {
			return nil, errors.New("objstore: list: server repeated a continuation token")
		}
		if pages >= maxPages {
			return nil, fmt.Errorf("objstore: list: more than %d pages under %q", maxPages, prefix)
		}
		seen[p.NextToken] = true
		token = p.NextToken
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Key < all[j].Key })
	return all, nil
}
