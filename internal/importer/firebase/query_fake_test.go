package firebase

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const snapshotTime = "2026-10-15T12:00:00.123456Z"

func (d *fakeDB) serveCount(w http.ResponseWriter, r *http.Request, parent string) {
	var body struct {
		ReadTime string `json:"readTime"`
		Q        struct {
			SQ struct {
				From []struct {
					CollectionID   string `json:"collectionId"`
					AllDescendants bool   `json:"allDescendants"`
				} `json:"from"`
			} `json:"structuredQuery"`
		} `json:"structuredAggregationQuery"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	selector := body.Q.SQ.From[0]
	n := 0
	if selector.AllDescendants && selector.CollectionID == "" && parent == "" {
		for path := range d.docs {
			n += d.existing(path)
		}
	} else {
		if body.ReadTime != snapshotTime {
			http.Error(w, "count snapshot missing", http.StatusBadRequest)
			return
		}
		path := selector.CollectionID
		if parent != "" {
			path = parent + "/" + path
		}
		n = d.existing(path)
	}
	writeJSON(w, []any{map[string]any{"readTime": snapshotTime, "result": map[string]any{"aggregateFields": map[string]any{"n": map[string]any{"integerValue": strconv.Itoa(n)}}}}})
}

func (d *fakeDB) serveQuery(w http.ResponseWriter, r *http.Request, project string) {
	var body struct {
		ReadTime string `json:"readTime"`
		Query    struct {
			Limit int `json:"limit"`
			From  []struct {
				All bool   `json:"allDescendants"`
				ID  string `json:"collectionId"`
			} `json:"from"`
			Start struct {
				Values []struct {
					Ref string `json:"referenceValue"`
				} `json:"values"`
				Before bool `json:"before"`
			} `json:"startAt"`
		} `json:"structuredQuery"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.ReadTime != snapshotTime || len(body.Query.From) != 1 || !body.Query.From[0].All || body.Query.From[0].ID != "" || body.Query.Limit != pageSize || body.Query.Start.Before {
		http.Error(w, "query not snapshot pinned or not all descendants", http.StatusBadRequest)
		return
	}
	prefix := "projects/" + project + "/databases/(default)/documents/"
	docs := map[string]fakeDoc{}
	var names []string
	for path, items := range d.docs {
		for _, doc := range items {
			if !doc.Missing {
				name := prefix + path + "/" + doc.ID
				docs[name] = doc
				names = append(names, name)
			}
		}
	}
	sort.Slice(names, func(i, j int) bool { return compareNames(names[i], names[j]) < 0 })
	if d.dropLast && len(names) > 0 {
		names = names[:(len(names)-1)/pageSize*pageSize]
	}
	start := ""
	if len(body.Query.Start.Values) > 0 {
		start = body.Query.Start.Values[0].Ref
	}
	out := []any{}
	for _, name := range names {
		if start != "" && compareNames(name, start) <= 0 {
			continue
		}
		out = append(out, map[string]any{"document": map[string]any{"name": name, "fields": docs[name].Fields}, "readTime": snapshotTime})
		if len(out) == body.Query.Limit {
			break
		}
	}
	if len(out) == 0 {
		out = append(out, map[string]any{"readTime": snapshotTime})
	}
	writeJSON(w, out)
}

// Firestore orders references by path segment, rather than the slash separator.
func compareNames(a, b string) int {
	aa, bb := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < min(len(aa), len(bb)); i++ {
		if c := strings.Compare(aa[i], bb[i]); c != 0 {
			return c
		}
	}
	return len(aa) - len(bb)
}
