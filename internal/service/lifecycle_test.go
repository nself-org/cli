package service

import (
	"os"
	"path/filepath"
	"testing"
)

// writeCompose writes a docker-compose.yml with the given body into a temp
// project dir and returns the dir.
func writeCompose(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestResolveService_StoragePrefersPlugin guards D-0451: when the compose file
// declares a `storage` service (the free storage plugin), the name `storage`
// must reach that container, not MinIO.
func TestResolveService_StoragePrefersPlugin(t *testing.T) {
	dir := writeCompose(t, "services:\n  minio:\n    image: minio/minio\n  storage:\n    image: nself/storage\n")
	got, err := resolveService("storage", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "storage" {
		t.Fatalf("resolveService(storage) = %q, want the plugin service %q", got, "storage")
	}
}

// TestResolveService_StorageFallsBackToMinio keeps the legacy meaning of the
// name when no storage plugin is wired into the stack.
func TestResolveService_StorageFallsBackToMinio(t *testing.T) {
	dir := writeCompose(t, "services:\n  minio:\n    image: minio/minio\n")
	got, err := resolveService("Storage", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "minio" {
		t.Fatalf("resolveService(Storage) = %q, want %q", got, "minio")
	}
}

// TestResolveService_StorageNoCompose falls back to minio when the compose
// file cannot be read, so the later compose call reports the real error.
func TestResolveService_StorageNoCompose(t *testing.T) {
	got, err := resolveService("storage", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != "minio" {
		t.Fatalf("got %q, want minio", got)
	}
}

func TestResolveService_OtherAliases(t *testing.T) {
	dir := writeCompose(t, "services:\n  storage:\n    image: x\n")
	for in, want := range map[string]string{"mail": "email", "mailpit": "email", "meilisearch": "search", "minio": "minio", "redis": "redis"} {
		got, err := resolveService(in, dir)
		if err != nil || got != want {
			t.Errorf("resolveService(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := resolveService("nope", dir); err == nil {
		t.Error("expected error for unknown service")
	}
}
