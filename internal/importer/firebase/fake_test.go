package firebase

// Purpose: an in-process Firestore REST and GCS JSON API double for the unit tests.
// Inputs: a fixture of typed documents. Outputs: an httptest server and a ready client.
// Constraints: no network; dropLast reproduces a pagination fault (the last page vanishes).
import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeDB struct {
	docs     map[string][]fakeDoc // collection path -> documents
	subs     map[string][]string  // parent path ("" = root) -> collection ids
	perPage  int
	dropLast bool
	objects  map[string]string // bucket object name -> body
	requests []string
}

type fakeDoc struct {
	ID     string
	Fields map[string]any // REST typed values
	// Missing marks a parent that holds no data (listed only with showMissing=true, never counted).
	Missing bool
}

func sv(s string) any                         { return map[string]any{"stringValue": s} }
func iv(n int64) any                          { return map[string]any{"integerValue": strconv.FormatInt(n, 10)} }
func dv(f float64) any                        { return map[string]any{"doubleValue": f} }
func bv(b bool) any                           { return map[string]any{"booleanValue": b} }
func tv(s string) any                         { return map[string]any{"timestampValue": s} }
func av(v ...any) any                         { return map[string]any{"arrayValue": map[string]any{"values": v}} }
func mv(f map[string]any) any                 { return map[string]any{"mapValue": map[string]any{"fields": f}} }
func nv() any                                 { return map[string]any{"nullValue": nil} }
func fields(kv map[string]any) map[string]any { return kv }

// fixtureDB has 2 top-level collections (users with the orders sub-collection, notes) and 12 documents.
func fixtureDB() *fakeDB {
	return &fakeDB{
		perPage: 2,
		docs: map[string][]fakeDoc{
			"users": {
				{ID: "u1", Fields: fields(map[string]any{"name": sv("Ann"), "age": iv(30), "active": bv(true), "score": dv(1.5), "joined": tv("2024-05-01T12:00:00.123456Z"), "tags": av(sv("a"), sv("b")), "profile": mv(map[string]any{"city": sv("Oslo"), "zip": iv(150)}), "nick": nv()})},
				{ID: "u2", Fields: fields(map[string]any{"name": sv("Bob"), "age": sv("unknown"), "active": bv(false), "score": dv(2), "joined": tv("2024-05-02T08:30:00Z")})},
				{ID: "u3", Fields: fields(map[string]any{"name": sv("Cy"), "age": iv(41), "active": bv(true), "nick": sv("cyc")})},
			},
			"notes": {
				{ID: "n1", Fields: fields(map[string]any{"title": sv("one"), "Title": sv("collides"), "id": sv("shadow")})},
				{ID: "n2", Fields: fields(map[string]any{"title": sv(""), "views": iv(0)})},
				{ID: "n3", Fields: fields(map[string]any{"title": sv("three"), "views": iv(9007199254740993)})},
				{ID: "n4", Fields: fields(map[string]any{})},
			},
			"users/u1/orders": {
				{ID: "o1", Fields: fields(map[string]any{"total": iv(10)})},
				{ID: "o2", Fields: fields(map[string]any{"total": iv(20)})},
				{ID: "o3", Fields: fields(map[string]any{"total": iv(30)})},
			},
			"users/u2/orders": {
				{ID: "o1", Fields: fields(map[string]any{"total": iv(5)})},
				{ID: "o2", Fields: fields(map[string]any{"total": iv(6)})},
			},
		},
		subs:    map[string][]string{"": {"users", "notes"}, "users/u1": {"orders"}, "users/u2": {"orders"}},
		objects: map[string]string{"avatars/ann.png": "PNGDATA-ANN", "docs/readme.txt": "hello"},
	}
}

func (d *fakeDB) handler(project, bucket string) http.Handler {
	prefix := "/v1/projects/" + project + "/databases/(default)/documents"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.requests = append(d.requests, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer owner" {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/storage/v1/b/"+bucket+"/o") {
			d.serveStorage(w, r, "/storage/v1/b/"+bucket+"/o")
			return
		}
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, prefix), "/")
		switch {
		case strings.HasSuffix(rest, ":listCollectionIds"):
			writeJSON(w, map[string]any{"collectionIds": d.subs[strings.TrimSuffix(rest, ":listCollectionIds")]})
		case strings.HasSuffix(rest, ":runAggregationQuery"):
			d.serveCount(w, r, strings.TrimSuffix(rest, ":runAggregationQuery"))
		case rest == ":runQuery":
			d.serveQuery(w, r, project)
		default:
			d.serveDocs(w, r, rest, prefix)
		}
	})
}

func (d *fakeDB) serveDocs(w http.ResponseWriter, r *http.Request, path, prefix string) {
	all := []fakeDoc{}
	for _, doc := range d.docs[path] {
		if !doc.Missing || r.URL.Query().Get("showMissing") == "true" {
			all = append(all, doc)
		}
	}
	if d.dropLast && len(all) > 0 {
		keep := (len(all) - 1) / d.perPage * d.perPage // drop the whole last page
		all = all[:keep]
	}
	start, _ := strconv.Atoi(r.URL.Query().Get("pageToken"))
	end := start + d.perPage
	out := map[string]any{}
	if end < len(all) {
		out["nextPageToken"] = strconv.Itoa(end)
	} else {
		end = len(all)
	}
	docs := []any{}
	for _, doc := range all[min(start, end):end] {
		entry := map[string]any{"name": "projects/p/databases/(default)/documents/" + path + "/" + doc.ID}
		if !doc.Missing {
			entry["fields"], entry["createTime"] = doc.Fields, "2024-01-01T00:00:00Z"
		}
		docs = append(docs, entry)
	}
	out["documents"] = docs
	writeJSON(w, out)
}

func (d *fakeDB) serveStorage(w http.ResponseWriter, r *http.Request, base string) {
	rest := strings.TrimPrefix(strings.TrimPrefix(r.URL.EscapedPath(), base), "/")
	if rest == "" {
		names := []string{"avatars/ann.png", "docs/readme.txt"}
		per, _ := strconv.Atoi(r.URL.Query().Get("maxResults"))
		start, _ := strconv.Atoi(r.URL.Query().Get("pageToken"))
		end := start + per
		out := map[string]any{}
		if end < len(names) {
			out["nextPageToken"] = strconv.Itoa(end)
		} else {
			end = len(names)
		}
		items := []any{}
		for _, n := range names[start:end] {
			items = append(items, map[string]any{"name": n, "size": strconv.Itoa(len(d.objects[n]))})
		}
		out["items"] = items
		writeJSON(w, out)
		return
	}
	name := strings.ReplaceAll(rest, "%2F", "/")
	body, ok := d.objects[name]
	if !ok || r.URL.Query().Get("alt") != "media" {
		http.NotFound(w, r)
		return
	}
	_, _ = w.Write([]byte(body))
}

func (d *fakeDB) existing(path string) int {
	n := 0
	for _, doc := range d.docs[path] {
		if !doc.Missing {
			n++
		}
	}
	return n
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// fakeClient starts the double and returns a Firestore reader (and GCS reader) wired to it.
func fakeClient(t *testing.T, d *fakeDB) (*firestore, *gcs, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(d.handler("demo-nself", "demo-bucket"))
	t.Cleanup(srv.Close)
	c := newClient(ownerToken{})
	c.backoff = func(int) time.Duration { return 0 }
	return &firestore{c: c, project: "demo-nself", base: srv.URL}, &gcs{c: c, base: srv.URL, bucket: "demo-bucket"}, srv
}
