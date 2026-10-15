package firebase

// Purpose: discover every document without a collection-id request per document.
// Inputs: a snapshot-pinned Firestore reader and a document callback.
// Outputs: bounded pages in resource-name order, including descendants of deleted parents.
// Constraints: any malformed or non-advancing page fails before an incomplete bundle is built.
import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type queryResult struct {
	Document struct {
		Name   string                     `json:"name"`
		Fields map[string]json.RawMessage `json:"fields"`
	} `json:"document"`
}

func (f *firestore) queryAll(ctx context.Context, fn func(string, document) error) error {
	last := ""
	for {
		query := map[string]any{
			"from": []map[string]any{{"allDescendants": true}}, "limit": pageSize,
			"orderBy": []map[string]any{{"field": map[string]any{"fieldPath": "__name__"}, "direction": "ASCENDING"}},
		}
		if last != "" {
			query["startAt"] = map[string]any{"values": []map[string]string{{"referenceValue": last}}, "before": false}
		}
		var page []queryResult
		body := map[string]any{"structuredQuery": query, "readTime": f.readTime}
		if err := f.c.doJSON(ctx, "POST", f.docsURL("")+":runQuery", body, &page); err != nil {
			return err
		}
		n := 0
		for _, row := range page {
			if row.Document.Name == "" {
				continue // Firestore sends a readTime-only result when the query is empty.
			}
			path, err := f.documentPath(row.Document.Name)
			if err != nil || !referenceAfter(row.Document.Name, last) {
				return errors.New("firestore query document name invalid or out of order")
			}
			parts := strings.Split(path, "/")
			if err = fn(path, document{ID: parts[len(parts)-1], Fields: row.Document.Fields}); err != nil {
				return err
			}
			last = row.Document.Name
			n++
		}
		if n < pageSize {
			return nil
		}
	}
}

// referenceAfter compares resource paths in Firestore's segment order.
func referenceAfter(name, cursor string) bool {
	if cursor == "" {
		return true
	}
	a, b := strings.Split(name, "/"), strings.Split(cursor, "/")
	for i := 0; i < min(len(a), len(b)); i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return len(a) > len(b)
}

func (f *firestore) documentPath(name string) (string, error) {
	prefix := "projects/" + f.project + "/databases/(default)/documents/"
	path := strings.TrimPrefix(name, prefix)
	parts := strings.Split(path, "/")
	if path == name || len(parts)%2 != 0 {
		return "", errors.New("firestore document path invalid")
	}
	for _, part := range parts {
		if part == "" || checkText(part) != nil {
			return "", errors.New("firestore document path invalid")
		}
	}
	return path, nil
}
