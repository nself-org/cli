package reconcile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvFileWithoutLine(t *testing.T) {
	const old = "# keep\nOTHER=\"one\nPOSTGRES_IMAGE=inside\ntwo\"\nexport POSTGRES_IMAGE=\"old\nimage\"\nPOSTGRES_IMAGE=duplicate\nEMPTY=\n"
	const want = "# keep\nOTHER=\"one\nPOSTGRES_IMAGE=inside\ntwo\"\nEMPTY=\n"
	got, err := EnvFileWithoutLine([]byte(old), "POSTGRES_IMAGE")
	if err != nil || string(got) != want {
		t.Fatalf("got %q, err %v", got, err)
	}
	again, err := EnvFileWithoutLine(got, "POSTGRES_IMAGE")
	if err != nil || string(again) != want {
		t.Fatalf("removal not idempotent: %q, %v", again, err)
	}
}

func TestPendingEnvRemovalMatchesDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("POSTGRES_IMAGE=new\nOTHER=keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	remove := map[string][]string{".env": {"POSTGRES_IMAGE"}}
	before, changed, err := pendingEnv(dir, nil, remove)
	if err != nil || !changed || string(before[".env"]) != "OTHER=keep\n" {
		t.Fatalf("pending removal: %q, changed=%v, err=%v", before, changed, err)
	}
	if err := os.WriteFile(path, before[".env"], 0o600); err != nil {
		t.Fatal(err)
	}
	after, changed, err := pendingEnv(dir, nil, remove)
	if err != nil || changed || string(before[".env"]) != string(after[".env"]) {
		t.Fatalf("after removal: %q, changed=%v, err=%v", after, changed, err)
	}
}
