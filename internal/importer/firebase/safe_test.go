package firebase

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/importer"
)

// extractErr runs Extract and returns the text of its user-visible failure.
func extractErr(t *testing.T, in importer.Input) string {
	t.Helper()
	_, err := (Source{}).Extract(context.Background(), in, filepath.Join(t.TempDir(), "stage"))
	var safe *importer.SafeError
	if !errors.As(err, &safe) {
		t.Fatalf("want an importer.SafeError, got %T %v", err, err)
	}
	return safe.What
}

func TestExtractFailuresKeepTheirFixedText(t *testing.T) {
	stubScratch(t)
	t.Run("empty project", func(t *testing.T) {
		d := fixtureDB()
		d.subs[""] = nil
		_, _, srv := fakeClient(t, d)
		if got := extractErr(t, extractInput(t, srv.URL, false)); got != "project has no Firestore collections" {
			t.Fatal(got)
		}
	})
	t.Run("NUL character", func(t *testing.T) {
		d := fixtureDB()
		d.docs["notes"] = append(d.docs["notes"], fakeDoc{ID: "n9", Fields: fields(map[string]any{"title": sv("a\x00b")})})
		_, _, srv := fakeClient(t, d)
		if got := extractErr(t, extractInput(t, srv.URL, false)); !strings.Contains(got, "document holds a NUL character") {
			t.Fatal(got)
		}
	})
	t.Run("HTTP status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "secret body", http.StatusForbidden)
		}))
		defer srv.Close()
		got := extractErr(t, extractInput(t, srv.URL, false))
		if !strings.Contains(got, "source API returned HTTP 403") || strings.Contains(got, "secret") {
			t.Fatal(got)
		}
	})
	t.Run("bad auth export date", func(t *testing.T) {
		_, _, srv := fakeClient(t, fixtureDB())
		in := extractInput(t, srv.URL, false)
		bad := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(bad, []byte(`{"users":[{"localId":"a","createdAt":"yesterday"}]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		in.Values["auth-export"] = bad
		if got := extractErr(t, in); !strings.Contains(got, "auth export timestamp is not epoch milliseconds") {
			t.Fatal(got)
		}
	})
}

func TestFailWithHidesUnknownCauses(t *testing.T) {
	var safe *importer.SafeError
	err := failWith("firestore read failed", errors.New("open /home/ann/.secret/key.json: permission denied"))
	if !errors.As(err, &safe) || safe.What != "firestore read failed" {
		t.Fatalf("%v", err)
	}
}
