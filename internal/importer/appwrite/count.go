package appwrite

import (
	"context"
	"encoding/json"
)

// userCount reads a cheap one-user query for Detect, then walks the list if its
// total reaches Appwrite's count cap. Inputs: source client. Outputs: exact user
// count. Constraints: capped totals cannot be trusted; no usage scope is required.
func (c *client) userCount(ctx context.Context) (int64, error) {
	p, err := c.listPageLimit(ctx, "/users", "users", "", 0, 1)
	if err != nil || p.Total < totalCap {
		return p.Total, err
	}
	var count int64
	_, err = c.each(ctx, "/users", "users", func(json.RawMessage) error {
		count++
		return nil
	})
	return count, err
}
