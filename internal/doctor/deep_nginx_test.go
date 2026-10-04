package doctor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTLSProject writes a minimal project .env and isolates the process env
// keys config.Load would overlay, so the test leaves nothing behind.
func writeTLSProject(t *testing.T, dir, env string) {
	t.Helper()
	for _, k := range []string{"ENV", "PROJECT_NAME", "SSL_MODE", "NGINX_FRONTED_BY"} {
		t.Setenv(k, "")
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
}

// requireSingleSkip asserts one skip result whose text never says "expires".
func requireSingleSkip(t *testing.T, got []CheckResult, wantIn string) {
	t.Helper()
	if len(got) != 1 || got[0].Status != "skip" {
		t.Fatalf("want one skip result, got %+v", got)
	}
	if strings.Contains(got[0].Message, "expires") || !strings.Contains(got[0].Message, wantIn) {
		t.Fatalf("skip message %q must contain %q and never %q", got[0].Message, wantIn, "expires")
	}
}

// TestServedCertChecksSkipsLocalTLS pins that SSL_MODE=local never probes.
func TestServedCertChecksSkipsLocalTLS(t *testing.T) {
	dir := t.TempDir()
	writeTLSProject(t, dir, "ENV=dev\nPROJECT_NAME=t\nSSL_MODE=local\n")
	requireSingleSkip(t, servedCertChecks(context.Background(), dir), "SSL_MODE=local")
}

// TestServedCertChecksFrontedUnresolved pins that a fronted project whose
// layout cannot be confirmed is skipped, never read from a guessed directory.
func TestServedCertChecksFrontedUnresolved(t *testing.T) {
	dir := t.TempDir()
	writeTLSProject(t, dir, "ENV=dev\nPROJECT_NAME=t\nSSL_MODE=letsencrypt\nNGINX_FRONTED_BY=nself-web\n")
	requireSingleSkip(t, servedCertChecks(context.Background(), dir), "NGINX_FRONTED_BY")
}

// TestNginxChecksSourceHasNoContainerLetsEncryptProbe pins that the deep check
// no longer looks inside the container for /etc/letsencrypt/live.
func TestNginxChecksSourceHasNoContainerLetsEncryptProbe(t *testing.T) {
	src, err := os.ReadFile("deep_nginx.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "/etc/letsencrypt/"+"live") {
		t.Fatal("deep_nginx.go still probes /etc/letsencrypt/live")
	}
}

// TestServedCertChecksLeavesEnvAlone pins that the deep check does not leave
// the project's .env cascade in this process's environment.
func TestServedCertChecksLeavesEnvAlone(t *testing.T) {
	dir := t.TempDir()
	writeTLSProject(t, dir, "ENV=dev\nPROJECT_NAME=envleak\nSSL_MODE=local\n")
	servedCertChecks(context.Background(), dir)
	if got := os.Getenv("PROJECT_NAME"); got != "" {
		t.Fatalf("PROJECT_NAME leaked into the process env: %q", got)
	}
}

// TestDeadAddr pins which probe errors short-circuit the remaining hosts.
func TestDeadAddr(t *testing.T) {
	if !DeadAddr(fmt.Errorf("probe: %w", &net.OpError{Op: "dial", Err: errors.New("refused")})) {
		t.Error("a failed dial is a dead address")
	}
	if DeadAddr(errors.New("tls: handshake failure")) || DeadAddr(nil) {
		t.Error("a handshake failure or no error is not a dead address")
	}
}
