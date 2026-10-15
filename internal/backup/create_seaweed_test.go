package backup

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

// seaweedCfg selects the v1.5 SeaweedFS store with no MinIO intent.
func seaweedCfg(t *testing.T, port string) *config.Config {
	t.Helper()
	compattest.Set(t, true)
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	t.Setenv("OBJECT_STORAGE_ENABLED", "true")
	t.Setenv("SEAWEEDFS_PORT", port)
	cfg := &config.Config{}
	cfg.Minio.S3AccessKey, cfg.Minio.S3SecretKey = "sw-access", "sw-secret"
	return cfg
}

// TestCreateObjectBackupSeaweed: the object backup reaches SeaweedFS on its published port
// with the S3 identity and mirrors the buckets (D-0683).
func TestCreateObjectBackupSeaweed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "Credential=sw-access/") {
			t.Error("request must use the S3 identity")
		}
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, `<ListAllMyBucketsResult><Buckets><Bucket><Name>uploads</Name></Bucket></Buckets></ListAllMyBucketsResult>`)
		case "/uploads":
			fmt.Fprint(w, `<ListBucketResult><Contents><Key>a.txt</Key><Size>2</Size></Contents></ListBucketResult>`)
		case "/uploads/a.txt":
			fmt.Fprint(w, "hi")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	cfg := seaweedCfg(t, u.Port())
	dir := t.TempDir()
	if err := createMinioBackup(context.Background(), cfg, dir, "stamp", "_tag"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "minio_stamp_tag", "uploads", "a.txt"))
	if err != nil || string(body) != "hi" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

// TestBackupAllIncludesObjectStore: `backup all` adds the object type for SeaweedFS too.
func TestBackupAllIncludesObjectStore(t *testing.T) {
	cfg := seaweedCfg(t, "")
	got := allBackupTypes(cfg)
	if len(got) != 3 || got[2] != BackupTypeMinio {
		t.Fatalf("all types = %v, want object type for SeaweedFS", got)
	}
}
