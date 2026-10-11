package compose

// custom_service_extras_overlay_test.go — CS_N_ENV_FILE reads honour the
// generator's env overlay (P7-LIVE-05): a plan of a pending `config set`
// sees the value a custom service will get; files outside it read the disk.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCustomServiceEnvFileOverlay(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.secrets"), []byte("TOKEN=disk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.dev"), []byte("OTHER=disk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := (&Generator{}).WithEnvOverlay(map[string][]byte{".env.secrets": []byte("TOKEN=pending\n")})
	vars, err := loadCustomServiceEnvFiles(dir, []string{".env.dev", "./.env.secrets"}, g.envOverlay)
	if err != nil {
		t.Fatal(err)
	}
	if vars["TOKEN"] != "pending" || vars["OTHER"] != "disk" {
		t.Fatalf("vars = %v", vars)
	}
	if _, err := loadCustomServiceEnvFiles(dir, []string{".env.secrets"}, map[string][]byte{".env.secrets": []byte("A=\"open\n")}); err == nil {
		t.Fatal("an unparsable overlay must fail like an unparsable file")
	}
}
