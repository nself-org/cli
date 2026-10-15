package appwrite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/portable"
)

func TestOverflowShrinksPageWithoutLosingRows(t *testing.T) {
	for _, cursor := range []bool{true, false} {
		t.Run(fmt.Sprint(cursor), func(t *testing.T) {
			f := &fake{docs: 120, t: t}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				limit, after, offset := cursorLimit(r)
				if limit > 50 {
					_, _ = io.CopyN(w, strings.NewReader(strings.Repeat(" ", maxPageBytes+1)), maxPageBytes+1)
					return
				}
				if !cursor && offset > 0 {
					after = fmt.Sprintf("n%d", offset-1)
				}
				f.documents(w, limit, after)
			}))
			defer srv.Close()
			c, err := newClient(srv.URL, "proj1", fakeKey)
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			total, err := c.walk(context.Background(), "/documents", "documents", cursor, func(raw json.RawMessage) error {
				var item struct {
					ID string `json:"$id"`
				}
				if err := json.Unmarshal(raw, &item); err != nil {
					return err
				}
				if seen[item.ID] {
					t.Errorf("duplicate %s", item.ID)
				}
				seen[item.ID] = true
				return nil
			})
			if err != nil || total != 120 || len(seen) != 120 {
				t.Fatalf("total=%d rows=%d err=%v", total, len(seen), err)
			}
		})
	}
}

func TestDetectCountsBeyondUserTotalCap(t *testing.T) {
	f := &fake{t: t}
	firstLimit := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/users" {
			f.handler(w, r)
			return
		}
		limit, after, _ := cursorLimit(r)
		if firstLimit == 0 {
			firstLimit = limit
		}
		start := 0
		if after != "" {
			_, _ = fmt.Sscanf(after, "u%d", &start)
			start++
		}
		items := []map[string]any{}
		for i := start; i < 5001 && len(items) < limit; i++ {
			items = append(items, map[string]any{"$id": fmt.Sprintf("u%d", i)})
		}
		writeList(w, "users", 5000, items)
	}))
	defer srv.Close()
	t.Setenv("APPWRITE_TEST_KEY", fakeKey)
	d, err := (Source{}).Detect(context.Background(), importer.Input{Values: map[string]string{
		"endpoint": srv.URL, "project": "proj1", "api-key-env": "APPWRITE_TEST_KEY",
	}})
	if err != nil || d.SourceUsers != 5001 || firstLimit != 1 {
		t.Fatalf("users=%d first limit=%d err=%v", d.SourceUsers, firstLimit, err)
	}
}

func TestStorageChecksServerSignature(t *testing.T) {
	for _, signature := range []string{"", "900150983cd24fb0d6963f7d28e17f72", "900150983CD24FB0D6963F7D28E17F72", "00000000000000000000000000000000", "invalid"} {
		t.Run(signature, func(t *testing.T) {
			f := &fake{t: t}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/storage/buckets/Files_One/files":
					writeList(w, "files", 1, []map[string]any{{"$id": "f1", "name": "a.txt", "sizeOriginal": 3, "signature": signature}})
				case "/v1/storage/buckets/Files_One/files/f1/download":
					_, _ = io.WriteString(w, "abc")
				default:
					f.handler(w, r)
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
			wantError := signature == "invalid" || strings.HasPrefix(signature, "0000")
			if (err != nil) != wantError {
				t.Fatalf("signature check: %v", err)
			}
		})
	}
}

func TestPagingStopsOnMalformedOrSingleItemOverflow(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		t.Run(fmt.Sprint(oversized), func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if oversized {
					_, _ = io.WriteString(w, strings.Repeat(" ", maxPageBytes+1))
				} else {
					_, _ = io.WriteString(w, "{invalid")
				}
			}))
			defer srv.Close()
			c, err := newClient(srv.URL, "proj1", fakeKey)
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.listPage(context.Background(), "/documents", "documents", "", 0)
			want := 1
			if oversized {
				want = 7
			} // 100, 50, 25, 12, 6, 3, 1
			if err == nil || requests != want {
				t.Fatalf("requests=%d want=%d err=%v", requests, want, err)
			}
		})
	}
}
