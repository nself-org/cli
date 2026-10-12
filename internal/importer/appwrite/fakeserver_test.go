package appwrite

// Purpose: an httptest Appwrite double shared by the unit tests.
// Constraints: obviously fake data only; the key is a synthetic constant.
import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const fakeKey = "fake-key-not-a-secret-0000"

type fake struct {
	docs        int  // documents in collection "notes"
	dropLast    bool // planted bug: the last page of a list is silently dropped
	t           *testing.T
	gotBadKey   bool
	downloadErr bool
}

// cursorLimit reads the limit and cursorAfter values from the queries[] parameters.
func cursorLimit(r *http.Request) (limit int, after string, offset int) {
	limit = 25
	for _, q := range r.URL.Query()["queries[]"] {
		var parsed struct {
			Method string `json:"method"`
			Values []any  `json:"values"`
		}
		if json.Unmarshal([]byte(q), &parsed) != nil || len(parsed.Values) == 0 {
			continue
		}
		switch parsed.Method {
		case "limit":
			limit = int(parsed.Values[0].(float64))
		case "cursorAfter":
			after = parsed.Values[0].(string)
		case "offset":
			offset = int(parsed.Values[0].(float64))
		}
	}
	return
}

func writeList(w http.ResponseWriter, key string, total int, items []map[string]any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"total": total, key: items})
}

func (f *fake) handler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Appwrite-Key") != fakeKey || r.Header.Get("X-Appwrite-Project") != "proj1" {
		f.gotBadKey = true
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	limit, after, offset := cursorLimit(r)
	p := strings.TrimPrefix(r.URL.Path, "/v1")
	switch {
	case p == "/health/version":
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "1.5.10"})
	case p == "/databases":
		writeList(w, "databases", 1, []map[string]any{{"$id": "main"}})
	case p == "/databases/main/collections":
		writeList(w, "collections", 2, []map[string]any{{"$id": "authors", "$permissions": []string{}}, {"$id": "notes", "$permissions": []string{`read("any")`}}})
	case p == "/databases/main/collections/authors/attributes":
		writeList(w, "attributes", 1, []map[string]any{{"key": "name", "type": "string", "status": "available"}})
	case p == "/databases/main/collections/notes/attributes":
		attrs := []map[string]any{{"key": "title", "type": "string", "status": "available"}, {"key": "views", "type": "integer", "status": "available"}, {"key": "author", "type": "relationship", "status": "available", "relatedCollection": "authors", "relationType": "manyToOne", "side": "parent"}}
		writeList(w, "attributes", len(attrs), attrs[min(offset, len(attrs)):])
	case p == "/databases/main/collections/authors/documents":
		writeList(w, "documents", 1, []map[string]any{{"$id": "a1", "$createdAt": "2026-01-01T00:00:00.000+00:00", "$updatedAt": "2026-01-01T00:00:00.000+00:00", "$permissions": []string{}, "name": "Ada"}})
	case p == "/databases/main/collections/notes/documents":
		f.documents(w, limit, after)
	case p == "/users":
		f.users(w)
	case p == "/storage/buckets":
		writeList(w, "buckets", 1, []map[string]any{{"$id": "Files_One"}})
	case p == "/storage/buckets/Files_One/files":
		writeList(w, "files", 2, []map[string]any{{"$id": "f1", "name": "a.txt"}, {"$id": "f2", "name": "dir/b.txt"}})
	case strings.HasPrefix(p, "/storage/buckets/Files_One/files/") && strings.HasSuffix(p, "/download"):
		_, _ = fmt.Fprintf(w, "bytes of %s", p)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fake) documents(w http.ResponseWriter, limit int, after string) {
	start := 0
	if after != "" {
		fmt.Sscanf(after, "n%d", &start)
		start++
	}
	items := []map[string]any{}
	for i := start; i < f.docs && len(items) < limit; i++ {
		items = append(items, map[string]any{"$id": fmt.Sprintf("n%d", i), "$createdAt": "2026-01-01T00:00:00.000+00:00", "$updatedAt": "2026-01-02T00:00:00.000+00:00", "$permissions": []string{}, "title": fmt.Sprintf("note \"%d\", with comma", i), "views": i, "author": map[string]any{"$id": "a1", "name": "Ada"}})
	}
	if f.dropLast && len(items) < limit && start > 0 {
		items = nil // the last (short) page is lost
	}
	writeList(w, "documents", f.docs, items)
}

func (f *fake) users(w http.ResponseWriter) {
	bcrypt := "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01234"
	writeList(w, "users", 4, []map[string]any{
		{"$id": "u1", "$createdAt": "2026-01-01T00:00:00.000+00:00", "registration": "2026-01-01T00:00:00.000+00:00", "name": "One", "email": "One@example.invalid", "password": bcrypt, "hash": "bcrypt", "status": true, "emailVerification": true},
		{"$id": "u2", "$createdAt": "2026-01-01T00:00:00.000+00:00", "registration": "2026-01-01T00:00:00.000+00:00", "email": "two@example.invalid", "password": "$argon2id$v=19$m=65536,t=4,p=1$fake$fake", "hash": "argon2", "status": true},
		{"$id": "u3", "$createdAt": "2026-01-01T00:00:00.000+00:00", "registration": "2026-01-01T00:00:00.000+00:00", "email": "three@example.invalid", "password": "fakescrypt", "hash": "scrypt", "status": false},
		{"$id": "u4", "$createdAt": "2026-01-01T00:00:00.000+00:00", "registration": "2026-01-01T00:00:00.000+00:00", "phone": "+15550100", "status": true},
	})
}

func newFake(t *testing.T, docs int) (*fake, *client) {
	t.Helper()
	f := &fake{docs: docs, t: t}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL, "proj1", fakeKey)
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}
