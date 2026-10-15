package commands

// Purpose: guard the HTTP-01 add flow against certbot writers (D-0460).
// Inputs: the served-stack fixture, saved certbot hooks and fake systemd.
// Outputs: refusal before issuance or retirement before replacing a certificate.
// Constraints: no host systemd, network or CA; POSIX fixtures skip on Windows.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/docker"
)

func TestSSLAddACMERefusesCertbotHooks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		dry, v15 bool
	}{
		{name: "issue"},
		{name: "dry run", dry: true},
		{name: "v1.5 default", v15: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, probed := addFix(t, nil)
			compattest.Set(t, tc.v15)
			le := useCertbotDir(t)
			sd := &fakeSystemd{load: "loaded", file: "enabled", active: "active"}
			useSystemd(t, sd)
			hook := filepath.Join(le, "renewal", "app.example.org.conf")
			// The served ssl tree is outside the project with NGINX_FRONTED_BY.
			acmeWriteFile(t, hook, []byte("[renewalparams]\n"+nselfDeployHook(f.served)))
			before := acmeTree(t, f.root)
			flags := []string{"--agree-tos"}
			if !tc.v15 {
				flags = append(flags, "--acme")
			}
			if tc.dry {
				flags = append(flags, "--dry-run")
			}
			_, err := f.runAdd("app.example.org", flags...)
			if err == nil || codeOf(err) != "E151" || !strings.Contains(err.Error(), hook+":2") {
				t.Fatalf("err=%v; want hook refusal naming %s:2", err, hook)
			}
			if acmeTree(t, f.root) != before || len(f.runs)+len(*probed)+sd.disables() != 0 {
				t.Fatal("hook refusal wrote, probed, issued or retired the timer")
			}
		})
	}
}

func TestSSLAddACMERetiresCertbotTimer(t *testing.T) {
	f, _ := addFix(t, nil)
	useCertbotDir(t)
	sd := &fakeSystemd{load: "loaded", file: "enabled", active: "active", disErr: errors.New("access denied")}
	useSystemd(t, sd)
	target := filepath.Join(f.ssl, "certificates", "app-example-org")
	acmeWriteFile(t, filepath.Join(target, "fullchain.pem"), []byte("old certificate"))
	acmeWriteFile(t, filepath.Join(target, "privkey.pem"), []byte("old key"))
	before := acmeTree(t, target)

	_, err := f.runAdd("app.example.org", "--acme", "--agree-tos")
	if err == nil || !strings.Contains(err.Error(), "systemctl disable --now "+certbotTimer) || len(f.runs) != 0 {
		t.Fatalf("denied retirement: err=%v lego runs=%d", err, len(f.runs))
	}
	if acmeTree(t, target) != before {
		t.Fatal("denied retirement changed the existing certificate pair")
	}
	if _, err := os.Readlink(target); err == nil {
		t.Fatal("denied retirement installed a generation")
	}

	sd.disErr, sd.calls = nil, nil
	out, err := f.runAdd("app.example.org", "--acme", "--dry-run")
	if err != nil || !strings.Contains(out, "would disable "+certbotTimer) || sd.disables() != 0 || acmeTree(t, target) != before {
		t.Fatalf("dry run: err=%v disables=%d\n%s", err, sd.disables(), out)
	}

	acmeD.run = func(ctx context.Context, spec docker.RunSpec) (string, string, error) {
		if sd.file != "disabled" || sd.active != "inactive" {
			t.Fatal("issuance started before certbot retirement")
		}
		return f.fakeLego(ctx, spec)
	}
	out, err = f.runAdd("app.example.org", "--acme", "--agree-tos")
	if err != nil || sd.disables() != 1 || !strings.Contains(out, "disabled "+certbotTimer) {
		t.Fatalf("add: err=%v disables=%d\n%s", err, sd.disables(), out)
	}
	if link, err := os.Readlink(target); err != nil || !strings.Contains(link, ".gen-") {
		t.Fatalf("add did not install a generation: %q %v", link, err)
	}
}

func TestSSLAddACMERechecksHooksAfterRetirement(t *testing.T) {
	f, _ := addFix(t, nil)
	le := useCertbotDir(t)
	hook := filepath.Join(le, "renewal", "app.example.org.conf")
	sd := &fakeSystemd{load: "loaded", file: "enabled", active: "active",
		onDisable: func() { acmeWriteFile(t, hook, []byte("[renewalparams]\n"+nselfDeployHook(f.served))) }}
	useSystemd(t, sd)
	out, err := f.runAdd("app.example.org", "--acme", "--agree-tos")
	if err == nil || !strings.Contains(err.Error(), hook+":2") || len(f.runs) != 0 || !strings.Contains(out, "was disabled before this failure") {
		t.Fatalf("hook saved during retirement: err=%v lego runs=%d\n%s", err, len(f.runs), out)
	}
}
