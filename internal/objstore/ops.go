package objstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
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

// EnsureBucket creates the bucket when it does not exist (HEAD, then PUT).
// Success when it already exists or another caller created it first.
func (c *Client) EnsureBucket(ctx context.Context, bucket string) error {
	if err := checkBucket(bucket); err != nil {
		return err
	}
	req, err := c.build(ctx, http.MethodHead, bucket, "", nil, emptySHA256)
	if err != nil {
		return err
	}
	resp, err := c.send(ctx, "head-bucket", req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK:
		return nil
	case resp.StatusCode != http.StatusNotFound:
		return &Error{Op: "head-bucket", Status: resp.StatusCode}
	}
	req, err = c.build(ctx, http.MethodPut, bucket, "", nil, emptySHA256)
	if err != nil {
		return err
	}
	req.ContentLength = 0
	resp, err = c.send(ctx, "create-bucket", req)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK {
		_ = resp.Body.Close()
		return nil
	}
	e := c.apiError("create-bucket", resp)
	var ae *Error
	if errors.As(e, &ae) && ae.Code == "BucketAlreadyOwnedByYou" {
		return nil
	}
	return e
}

// Put uploads size bytes from body as bucket/key.
//
// Over plain http to a loopback endpoint the body streams with
// UNSIGNED-PAYLOAD. Otherwise the payload is hashed first and signed: a
// seekable body is hashed and rewound, any other body is spooled to a
// temporary file (removed before return).
func (c *Client) Put(ctx context.Context, bucket, key string, body io.Reader, size int64) error {
	if err := checkBucket(bucket); err != nil {
		return err
	}
	if err := checkKey(key); err != nil {
		return err
	}
	if size < 0 {
		return errors.New("objstore: size must be known and non-negative")
	}
	u, err := c.endpoint()
	if err != nil {
		return err
	}
	payload, rd, cleanup, err := preparePayload(body, size, isLoopbackHTTP(u))
	if err != nil {
		return err
	}
	defer cleanup()
	req, err := c.build(ctx, http.MethodPut, bucket, key, nil, payload)
	if err != nil {
		return err
	}
	req.ContentLength = size
	if size == 0 {
		req.Body = http.NoBody
	} else {
		req.Body = io.NopCloser(rd)
	}
	resp, err := c.send(ctx, "put", req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return c.apiError("put", resp)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))
	return resp.Body.Close()
}

// preparePayload returns the payload hash for the signature and a reader
// positioned at the start of exactly size bytes.
func preparePayload(body io.Reader, size int64, streamUnsigned bool) (string, io.Reader, func(), error) {
	noop := func() {}
	if body == nil || size == 0 {
		return emptySHA256, http.NoBody, noop, nil
	}
	lr := io.LimitReader(body, size+1)
	if streamUnsigned {
		return unsignedPayload, &exactReader{r: lr, want: size}, noop, nil
	}
	rs, ok := body.(io.ReadSeeker)
	cleanup := noop
	if ok {
		start, err := rs.Seek(0, io.SeekCurrent)
		if err != nil {
			return "", nil, noop, fmt.Errorf("objstore: seek body: %w", err)
		}
		h := sha256.New()
		n, err := io.Copy(h, io.LimitReader(rs, size+1))
		if err != nil || n != size {
			return "", nil, noop, fmt.Errorf("objstore: body is %d bytes, size says %d (%v)", n, size, err)
		}
		if _, err := rs.Seek(start, io.SeekStart); err != nil {
			return "", nil, noop, fmt.Errorf("objstore: rewind body: %w", err)
		}
		return hex.EncodeToString(h.Sum(nil)), io.LimitReader(rs, size), cleanup, nil
	}
	f, err := os.CreateTemp("", "nself-objstore-*")
	if err != nil {
		return "", nil, noop, fmt.Errorf("objstore: spool body: %w", err)
	}
	cleanup = func() { _ = f.Close(); _ = os.Remove(f.Name()) }
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), lr)
	if err != nil || n != size {
		cleanup()
		return "", nil, noop, fmt.Errorf("objstore: body is %d bytes, size says %d (%v)", n, size, err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return "", nil, noop, fmt.Errorf("objstore: rewind spool: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), f, cleanup, nil
}

// exactReader fails if the stream is not exactly want bytes (the LimitReader
// below it allows one extra byte so an over-long body is detected).
type exactReader struct {
	r    io.Reader
	want int64
	n    int64
}

func (e *exactReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	e.n += int64(n)
	if e.n > e.want {
		return n, errors.New("objstore: body is longer than size")
	}
	if err == io.EOF && e.n != e.want {
		return n, io.ErrUnexpectedEOF
	}
	return n, err
}

// Get opens bucket/key. The caller closes the body.
func (c *Client) Get(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	if err := checkBucket(bucket); err != nil {
		return nil, err
	}
	if err := checkKey(key); err != nil {
		return nil, err
	}
	req, err := c.build(ctx, http.MethodGet, bucket, key, nil, emptySHA256)
	if err != nil {
		return nil, err
	}
	resp, err := c.send(ctx, "get", req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.apiError("get", resp)
	}
	return resp.Body, nil
}

type listResult struct {
	IsTruncated bool   `xml:"IsTruncated"`
	NextToken   string `xml:"NextContinuationToken"`
	Contents    []struct {
		Key  string `xml:"Key"`
		Size int64  `xml:"Size"`
		ETag string `xml:"ETag"`
	} `xml:"Contents"`
}

// ListPage returns one ListObjectsV2 page for prefix, starting at token ("" for
// the first page). The continuation token is URL-encoded on the wire.
func (c *Client) ListPage(ctx context.Context, bucket, prefix, token string) (Page, error) {
	if err := checkBucket(bucket); err != nil {
		return Page{}, err
	}
	q := [][2]string{{"list-type", "2"}}
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
		p.Objects = append(p.Objects, Object{Key: o.Key, Size: o.Size, ETag: o.ETag})
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
// error, not an endless loop.
func (c *Client) List(ctx context.Context, bucket, prefix string) ([]Object, error) {
	var all []Object
	seen := map[string]bool{}
	token := ""
	for {
		p, err := c.ListPage(ctx, bucket, prefix, token)
		if err != nil {
			return nil, err
		}
		all = append(all, p.Objects...)
		if p.NextToken == "" {
			break
		}
		if seen[p.NextToken] {
			return nil, errors.New("objstore: list: server repeated a continuation token")
		}
		seen[p.NextToken] = true
		token = p.NextToken
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Key < all[j].Key })
	return all, nil
}
