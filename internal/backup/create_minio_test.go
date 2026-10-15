package backup

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestCreateMinioBackupWithoutHostMC(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "Credential=backup-user/") {
			t.Error("S3 request must use configured credentials")
		}
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, `<ListAllMyBucketsResult><Buckets><Bucket><Name>uploads</Name></Bucket><Bucket><Name>empty</Name></Bucket></Buckets></ListAllMyBucketsResult>`)
		case "/empty":
			fmt.Fprint(w, `<ListBucketResult/>`)
		case "/uploads":
			if r.URL.Query().Get("continuation-token") == "next" {
				fmt.Fprintf(w, `<ListBucketResult><Contents><Key>second.txt</Key><Size>%d</Size></Contents><Contents><Key>photos/</Key><Size>0</Size></Contents></ListBucketResult>`, len("object:/uploads/second.txt"))
			} else {
				fmt.Fprintf(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>next</NextContinuationToken><Contents><Key>nested/hello%%2Bworld.txt</Key><Size>%d</Size></Contents><EncodingType>url</EncodingType></ListBucketResult>`, len("object:/uploads/nested/hello+world.txt"))
			}
		case "/uploads/nested/hello+world.txt", "/uploads/second.txt":
			fmt.Fprint(w, "object:"+r.URL.Path)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := &config.Config{Minio: config.MinioConfig{Enabled: true, Port: port, RootUser: "backup-user", RootPassword: "backup-secret"}}
	dir := t.TempDir()
	if err := createMinioBackup(context.Background(), cfg, dir, "stamp", "_tag"); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "minio_stamp_tag")
	for _, key := range []string{"nested/hello+world.txt", "second.txt"} {
		path := filepath.Join(root, "uploads", key)
		body, err := os.ReadFile(path)
		if err != nil || string(body) != "object:/uploads/"+key {
			t.Fatalf("%s: body=%q err=%v", key, body, err)
		}
		info, _ := os.Stat(path)
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 { // Windows reports 0666 for every file
			t.Errorf("object permissions = %o", info.Mode().Perm())
		}
	}
	if info, err := os.Stat(filepath.Join(root, "empty")); err != nil || !info.IsDir() {
		t.Fatalf("empty bucket missing: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "uploads", "photos")); err != nil || !info.IsDir() {
		t.Fatalf("directory marker missing: %v", err)
	}
}

func TestCreateMinioBackupFailureLeavesNoPartialBackup(t *testing.T) {
	for _, key := range []string{"../escape", "/absolute", "a//b", `a\b`, "download-fails", "nonempty-marker/", "size-changed", "./"} {
		t.Run(key, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
					fmt.Fprint(w, `<ListAllMyBucketsResult><Buckets><Bucket><Name>uploads</Name></Bucket></Buckets></ListAllMyBucketsResult>`)
				case "/uploads":
					size := 1
					if key == "./" {
						size = 0
					}
					fmt.Fprintf(w, `<ListBucketResult><Contents><Key>%s</Key><Size>%d</Size></Contents></ListBucketResult>`, key, size)
				default:
					if key == "size-changed" {
						fmt.Fprint(w, "longer than listed")
					} else {
						http.Error(w, "download failed", http.StatusInternalServerError)
					}
				}
			}))
			defer srv.Close()
			u, _ := url.Parse(srv.URL)
			port, _ := strconv.Atoi(u.Port())
			cfg := &config.Config{Minio: config.MinioConfig{Enabled: true, Port: port, RootUser: "user", RootPassword: "secret"}}
			dir := t.TempDir()
			if err := createMinioBackup(context.Background(), cfg, dir, "stamp", ""); err == nil {
				t.Fatal("expected backup failure")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("incomplete backup left behind: %v, %v", entries, err)
			}
		})
	}
}
