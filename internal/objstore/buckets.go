package objstore

// Purpose: enumerate buckets using the same signed S3 transport as object operations.
// Inputs: request context and configured Client. Outputs: validated bucket names.
// Constraints: bounds the XML response; never accepts server-provided filesystem paths.

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
)

// ListBuckets returns all buckets visible to the configured credentials.
func (c *Client) ListBuckets(ctx context.Context) ([]string, error) {
	req, err := c.build(ctx, http.MethodGet, "", "", nil, emptySHA256)
	if err != nil {
		return nil, err
	}
	resp, err := c.send(ctx, "list-buckets", req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.apiError("list-buckets", resp)
	}
	defer func() { _ = resp.Body.Close() }()
	var result struct {
		XMLName xml.Name `xml:"ListAllMyBucketsResult"`
		Buckets []struct {
			Name string `xml:"Name"`
		} `xml:"Buckets>Bucket"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, maxListBody)).Decode(&result); err != nil {
		return nil, fmt.Errorf("objstore: list-buckets: cannot parse response: %w", err)
	}
	buckets := make([]string, 0, len(result.Buckets))
	for _, b := range result.Buckets {
		if err := checkBucket(b.Name); err != nil {
			return nil, err
		}
		buckets = append(buckets, b.Name)
	}
	return buckets, nil
}
