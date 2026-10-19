package commands

// ssl_conf_restore_test.go: a custom-domain conf nginx rejects is taken out
// again (D-0412), on the snapshot helper and through `trust ssl add --acme`.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfSnapshotRestore(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before *string
	}{
		{"new conf is removed", nil},
		{"existing conf gets its bytes back", ptr("# hand edited\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			confDir := filepath.Join(work, "nginx", "conf.d")
			if err := os.MkdirAll(confDir, 0o750); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(confDir, "custom-app-example-org.conf")
			if tc.before != nil {
				if err := os.WriteFile(path, []byte(*tc.before), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			snap, err := snapshotCustomDomainConf(work, "", "app.example.org")
			if err != nil {
				t.Fatal(err)
			}
			if err := writeCustomDomainConfServed(work, "", "app.example.org", ""); err != nil {
				t.Fatal(err)
			}
			if err := snap.restore(); err != nil {
				t.Fatal(err)
			}
			got, rerr := os.ReadFile(path)
			switch {
			case tc.before == nil && !errors.Is(rerr, os.ErrNotExist):
				t.Errorf("conf still there after restore: %q %v", got, rerr)
			case tc.before != nil && string(got) != *tc.before:
				t.Errorf("conf = %q, want %q", got, *tc.before)
			}
		})
	}
}

func TestSSLAddACMENginxRejectsConf(t *testing.T) {
	f, _ := addFix(t, nil)
	conf := filepath.Join(f.served, "nginx", "conf.d", "custom-app-example-org.conf")
	// nginx -t fails only once the new conf is on disk, as it would for a bad --upstream.
	acmeD.exec = func(_ context.Context, _ string, cmd []string) (string, string, error) {
		f.execs = append(f.execs, cmd)
		if _, err := os.Stat(conf); err == nil && cmd[len(cmd)-1] == "-t" {
			return "", "nginx: [emerg] host not found in upstream", errors.New("exit status 1")
		}
		return "", "", nil
	}
	_, err := f.runAdd("app.example.org", "--acme", "--agree-tos", "--upstream=bad_host:3000")
	if err == nil || !strings.Contains(err.Error(), "the new conf was removed") {
		t.Fatalf("want a refusal naming the removed conf, got %v", err)
	}
	if _, serr := os.Stat(conf); !errors.Is(serr, os.ErrNotExist) {
		t.Errorf("rejected conf left behind: %v", serr)
	}
}
