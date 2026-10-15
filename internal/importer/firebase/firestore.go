package firebase

// Purpose: probe collection ids and pin independent server-side COUNTs to one snapshot.
// Inputs: project id and base URL (the emulator and real projects share this interface).
// Outputs: documents streamed to a callback, counts. Constraints: a parent path is "" for the
// root or "users/u1"; ids are escaped per path segment; the count uses a different API than the
// document listing so a pagination fault shows up as a count difference.
import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

const pageSize = 40

type firestore struct {
	c        *client
	project  string
	base     string
	readTime string
}

func (f *firestore) docsURL(path string) string {
	u := f.base + "/v1/projects/" + url.PathEscape(f.project) + "/databases/(default)/documents"
	for _, seg := range strings.Split(path, "/") {
		if seg != "" {
			u += "/" + url.PathEscape(seg)
		}
	}
	return u
}

// collectionIDs lists the sub-collection ids under parent ("" = database root).
func (f *firestore) collectionIDs(ctx context.Context, parent string) ([]string, error) {
	var ids []string
	token := ""
	for {
		body := map[string]any{"pageSize": 100}
		if token != "" {
			body["pageToken"] = token
		}
		var page struct {
			CollectionIDs []string `json:"collectionIds"`
			NextPageToken string   `json:"nextPageToken"`
		}
		if err := f.c.doJSON(ctx, "POST", f.docsURL(parent)+":listCollectionIds", body, &page); err != nil {
			return nil, err
		}
		ids = append(ids, page.CollectionIDs...)
		if page.NextPageToken == "" {
			return ids, nil
		}
		token = page.NextPageToken
	}
}

type document struct {
	ID     string
	Fields map[string]json.RawMessage
}

// count counts a collection, or all descendants at the root when collectionID is empty.
func (f *firestore) count(ctx context.Context, parent, collectionID string) (int64, error) {
	selector := map[string]any{"collectionId": collectionID}
	if collectionID == "" {
		selector = map[string]any{"allDescendants": true}
	}
	body := map[string]any{"structuredAggregationQuery": map[string]any{
		"structuredQuery": map[string]any{"from": []map[string]any{selector}},
		"aggregations":    []map[string]any{{"alias": "n", "count": map[string]any{}}},
	}}
	if f.readTime != "" {
		body["readTime"] = f.readTime
	}
	var out []struct {
		ReadTime string `json:"readTime"`
		Result   struct {
			AggregateFields map[string]struct {
				IntegerValue json.Number `json:"integerValue"`
			} `json:"aggregateFields"`
		} `json:"result"`
	}
	if err := f.c.doJSON(ctx, "POST", f.docsURL(parent)+":runAggregationQuery", body, &out); err != nil {
		return 0, err
	}
	for _, r := range out {
		if v, ok := r.Result.AggregateFields["n"]; ok {
			if f.readTime == "" {
				if r.ReadTime == "" {
					return 0, errors.New("firestore snapshot readTime missing")
				}
				f.readTime = r.ReadTime
			}
			n, err := v.IntegerValue.Int64()
			if err != nil || n < 0 {
				return 0, errors.New("document count invalid")
			}
			return n, nil
		}
	}
	return 0, errors.New("document count missing")
}
