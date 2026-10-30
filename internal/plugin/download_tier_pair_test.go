package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestTierPairDownloadNamesResolvedTier(t *testing.T) {
	for _, paid := range []bool{false, true} {
		want := "free"
		if paid {
			want = "licensed"
		}
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.URL.Path != "/plugins/notify/download" || r.URL.Query().Get("tier") != want {
				t.Errorf("unexpected download route: %s", r.URL)
			}
			_, _ = w.Write([]byte("archive"))
		}))
		t.Setenv("NSELF_PING_API_URL", srv.URL)
		path, kind, err := downloadPluginPackageForTier(context.Background(), "notify", "1.0.0", "", "", paid, true)
		srv.Close()
		if err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(path)
		if calls != 1 || kind != ArtifactKindSource {
			t.Fatalf("calls=%d kind=%s", calls, kind)
		}
	}
}
