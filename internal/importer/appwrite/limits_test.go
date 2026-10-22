package appwrite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/portable"
)

func TestOversizedPages(t *testing.T) {
	for _, budget := range []int64{500, 20} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				limit, after, _ := cursorLimit(r)
				start := 0
				if after != "" {
					_, _ = fmt.Sscanf(after, "n%d", &start)
					start++
				}
				items := []map[string]any{}
				for i := start; i < 7 && len(items) < limit; i++ {
					items = append(items, map[string]any{"$id": fmt.Sprintf("n%d", i), "text": strings.Repeat("x", 80)})
				}
				writeList(w, "documents", 7, items)
			}))
			defer srv.Close()
			c, err := newClient(srv.URL, "proj1", fakeKey)
			if err != nil {
				t.Fatal(err)
			}
			c.pageBytes = budget
			seen := 0
			total, err := c.each(context.Background(), "/documents", "documents", func(json.RawMessage) error { seen++; return nil })
			if budget == 20 {
				if err == nil {
					t.Fatal("oversized single item accepted")
				}
				return
			}
			if err != nil || seen != 7 || total != 7 {
				t.Fatalf("seen=%d total=%d err=%v", seen, total, err)
			}
		})
	}
}

func TestStorageSignatures(t *testing.T) {
	for _, sig := range []string{"", "321c3cf486ed509164edec1e1981fec8", "00000000000000000000000000000000", "invalid"} {
		t.Run("signature="+sig, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/storage/buckets":
					writeList(w, "buckets", 1, []map[string]any{{"$id": "bucket"}})
				case "/v1/storage/buckets/bucket/files":
					writeList(w, "files", 1, []map[string]any{{"$id": "file", "name": "file.txt", "signature": sig, "sizeOriginal": 7}})
				default:
					_, _ = w.Write([]byte("payload"))
				}
			}))
			defer srv.Close()
			c, err := newClient(srv.URL, "proj1", fakeKey)
			if err != nil {
				t.Fatal(err)
			}
			w, err := portable.NewWriter(filepath.Join(t.TempDir(), "bundle"))
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.exportStorage(context.Background(), w)
			wantErr := sig != "" && sig != "321c3cf486ed509164edec1e1981fec8"
			if (err != nil) != wantErr {
				t.Fatalf("want error %v, got %v", wantErr, err)
			}
		})
	}
}

func TestDetectionUsesCheapUserCount(t *testing.T) {
	f := &fake{t: t}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/users" {
			limit, _, _ := cursorLimit(r)
			if limit != 1 {
				t.Errorf("user count fetched %d users, want one", limit)
			}
		}
		f.handler(w, r)
	}))
	defer srv.Close()
	d, err := (Source{}).Detect(context.Background(), importer.Input{Values: map[string]string{"endpoint": srv.URL, "project": "proj1", "api-key-env": "AW_TEST_KEY"}, Env: func(string) string { return fakeKey }})
	if err != nil || d.SourceUsers != 4 {
		t.Fatalf("count=%d error=%v", d.SourceUsers, err)
	}
}
