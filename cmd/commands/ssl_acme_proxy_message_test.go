package commands

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
)

func TestAcmeTraefikServedMismatchNamesProxy(t *testing.T) {
	f := newACMEFix(t)
	oldWait := acmeVerifyWait
	acmeVerifyWait = time.Millisecond
	t.Cleanup(func() { acmeVerifyWait = oldWait })
	acmeD.probe = func(context.Context, string, string, time.Duration) (ssl.ServedCert, error) {
		return ssl.ServedCert{SHA256: "different"}, nil
	}
	r := &acmeRun{provider: build.ProviderTraefik, cfg: &config.Config{}, workdir: f.project, res: &acme.Resolution{SSLDir: f.ssl, Root: f.served}}
	cert, _ := acmeTestCert(t, []string{"api.example.org"}, time.Now().Add(time.Hour))
	err := r.verifier(acme.Lineage{Domains: []string{"api.example.org"}, Targets: []string{"certificates/api-example-org"}}, cert)(context.Background())
	if err == nil || !strings.Contains(err.Error(), "traefik at ") || strings.Contains(err.Error(), "nginx") {
		t.Fatalf("wrong proxy in served-certificate failure: %v", err)
	}
}
