package firebase

// Purpose: read Firestore through the REST API: collection ids, paginated documents, and the
// server-side COUNT aggregation that serves as the independent source count.
// Inputs: project id and base URL (the emulator and real projects share this interface).
// Outputs: documents streamed to a callback, counts. Constraints: a parent path is "" for the
// root or "users/u1"; ids are escaped per path segment; the count uses a different API than the
// document listing so a pagination fault shows up as a count difference.
import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

const pageSize = 300

type firestore struct {
	c       *client
	project string
	base    string
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
	// Missing is true for a document that holds no data of its own and exists only as the parent of
	// sub-collections (Firestore keeps those after the parent is deleted). It is listed with
	// showMissing=true so its sub-collections are still walked; it is never a table row.
	Missing bool
}

// listDocs streams every document of a collection path ("users" or "users/u1/orders"), missing
// parents included, to fn and returns how many existing documents it saw.
func (f *firestore) listDocs(ctx context.Context, collection string, fn func(document) error) (int64, error) {
	var n int64
	token := ""
	for {
		u := f.docsURL(collection) + "?showMissing=true&pageSize=" + strconv.Itoa(pageSize)
		if token != "" {
			u += "&pageToken=" + url.QueryEscape(token)
		}
		var page struct {
			Documents []struct {
				Name       string                     `json:"name"`
				Fields     map[string]json.RawMessage `json:"fields"`
				CreateTime string                     `json:"createTime"`
			} `json:"documents"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := f.c.getJSON(ctx, u, &page); err != nil {
			return n, err
		}
		for _, d := range page.Documents {
			id := d.Name[strings.LastIndex(d.Name, "/")+1:]
			if id == "" {
				return n, errors.New("document name invalid")
			}
			if err := checkText(id); err != nil {
				return n, err
			}
			missing := d.CreateTime == "" && len(d.Fields) == 0
			if err := fn(document{ID: id, Fields: d.Fields, Missing: missing}); err != nil {
				return n, err
			}
			if !missing {
				n++
			}
		}
		if page.NextPageToken == "" {
			return n, nil
		}
		token = page.NextPageToken
	}
}

// count runs a COUNT aggregation over the collection id under parent.
func (f *firestore) count(ctx context.Context, parent, collectionID string) (int64, error) {
	body := map[string]any{"structuredAggregationQuery": map[string]any{
		"structuredQuery": map[string]any{"from": []map[string]any{{"collectionId": collectionID}}},
		"aggregations":    []map[string]any{{"alias": "n", "count": map[string]any{}}},
	}}
	var out []struct {
		Result struct {
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
			n, err := v.IntegerValue.Int64()
			if err != nil {
				return 0, errors.New("document count invalid")
			}
			return n, nil
		}
	}
	return 0, errors.New("document count missing")
}
